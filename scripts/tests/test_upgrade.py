"""Exercise upgrade control flow against an isolated Docker CLI simulator.
No daemon, registry, live credentials or production containers are touched.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/upgrade.sh'
OLD = 'sha256:' + 'a' * 64
NEW = 'sha256:' + 'b' * 64
MOCK = r'''#!/usr/bin/env python3
import json, os, re, sys
from pathlib import Path
path=Path(os.environ['UPGRADE_TEST_STATE'])
state=json.loads(path.read_text())
args=sys.argv[1:];state['calls'].append(args)
def finish(text='',code=0):
 path.write_text(json.dumps(state));print(text);sys.exit(code)
if args==['info'] or args==['compose','version']:finish('ok')
if args[0]=='inspect':
 target=args[-1]
 if target not in ['x-x-codex2api','old-container','new-container']:finish('unknown container',1)
 fmt=args[args.index('--format')+1]
 labels={'com.docker.compose.project':'x-x-codex2api','com.docker.compose.service':'codex2api','com.docker.compose.project.working_dir':state['workdir'],'com.docker.compose.project.config_files':state['compose_file'],'com.docker.compose.project.environment_file':state.get('env_file','')}
 if '.Config.Labels' in fmt:finish(labels.get(re.search(r'"([^"]+)"',fmt).group(1),''))
 if fmt=='{{.Image}}':finish(state['current_image'])
 if fmt=='{{.Id}}':finish(state['cid'])
 if fmt=='{{.State.Running}}':finish('true')
 finish('bad format',1)
if args[:2]==['image','inspect']:
 ref=args[-1]
 image=state['tags'].get(ref)
 if not image:finish('image missing',1)
 finish(image if '--format' in args else '[]')
if args[0]=='tag':
 if args[1] not in state['tags'].values():finish('missing source',1)
 state['tags'][args[2]]=args[1];finish()
if args[0]=='exec':
 if state.get('unhealthy'):finish('health failed',1)
 finish(json.dumps({'status':'ok','build_version':'sha-test'},separators=(',',':')))
if args[0]=='compose':
 if 'config' in args:finish('codex2api\npostgres\nredis')
 if 'ps' in args:finish('old-container\nnew-container' if state.get('scaled') else state['cid'])
 if 'pull' in args:
  if state.get('pull_fail'):finish('pull denied',1)
  overrides=[args[i+1] for i,x in enumerate(args[:-1]) if x=='-f']
  ref=json.loads(Path(overrides[-1]).read_text())['services']['codex2api']['image']
  state['tags'][ref]=state['new_image'];finish('pulled')
 if 'up' in args:
  if state.get('up_fail'):finish('up failed',1)
  overrides=[args[i+1] for i,x in enumerate(args[:-1]) if x=='-f']
  ref=json.loads(Path(overrides[-1]).read_text())['services']['codex2api']['image']
  state['current_image']=state['tags'][ref] if not state.get('wrong_image') else state['old_image'];state['cid']='new-container';finish('recreated')
finish('unexpected command '+str(args),1)
'''


class UpgradeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='core upgrade ')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.compose = self.root / 'docker-compose.prod.yml'
        self.compose.write_text('services:\n  codex2api:\n    image: local:old\n')
        (self.root / '.env').write_text('ADMIN_SECRET=fixture-never-print\n')
        bin_dir = self.root / 'bin'
        bin_dir.mkdir()
        executable = bin_dir / 'docker'
        executable.write_text(MOCK)
        executable.chmod(0o755)
        # Avoid real waits while keeping the script's actual deadline behavior.
        sleeper = bin_dir / 'sleep'
        sleeper.write_text('#!/bin/sh\nexit 0\n')
        sleeper.chmod(0o755)
        self.state_file = self.root / 'state.json'
        self.state = {'workdir': str(self.root), 'compose_file': str(self.compose), 'cid': 'old-container', 'current_image': OLD, 'old_image': OLD, 'new_image': NEW, 'tags': {'local:old': OLD}, 'calls': []}
        self.save()
        self.env = {**os.environ, 'PATH': str(bin_dir) + os.pathsep + os.environ['PATH'], 'UPGRADE_TEST_STATE': str(self.state_file)}

    def save(self):
        self.state_file.write_text(json.dumps(self.state))

    def run_script(self, *extra, container=False):
        args = ['--container', 'x-x-codex2api'] if container else ['--file', str(self.compose), '--project', 'x-x-codex2api', '--service', 'codex2api']
        result = subprocess.run(['bash', str(SCRIPT), *args, *extra], cwd=self.root, env=self.env, text=True, capture_output=True, timeout=15)
        self.state = json.loads(self.state_file.read_text())
        self.assertNotIn('fixture-never-print', result.stdout + result.stderr)
        return result

    def up_calls(self):
        return [call for call in self.state['calls'] if call[0] == 'compose' and 'up' in call]

    def test_upgrade_preserves_source_and_targets_only_application(self):
        original = self.compose.read_bytes()
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.state['current_image'], NEW)
        self.assertEqual(self.compose.read_bytes(), original)
        self.assertEqual(len(self.up_calls()), 1)
        command = self.up_calls()[0]
        self.assertEqual(command[-1], 'codex2api')
        for option in ['--no-deps', '--no-build', '--pull', 'never']:
            self.assertIn(option, command)
        self.assertIn('x-x-codex2api', command)
        self.assertFalse(any(word in call for call in self.state['calls'] for word in ['down', 'prune', 'postgres', 'redis']))
        self.assertTrue(any(image == OLD for tag, image in self.state['tags'].items() if tag.startswith('codex2apicore-rollback:')))
        pull = next(i for i, call in enumerate(self.state['calls']) if 'pull' in call)
        up = next(i for i, call in enumerate(self.state['calls']) if 'up' in call)
        self.assertLess(pull, up)

    def test_container_autodetection_and_custom_env_file(self):
        env_file = self.root / 'custom.env'
        env_file.write_text('VALUE=fixture-never-print\n')
        self.state['env_file'] = str(env_file)
        self.save()
        result = self.run_script(container=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('--env-file', self.up_calls()[0])
        self.assertIn(str(env_file), self.up_calls()[0])

    def test_container_repeated_upgrade_does_not_duplicate_managed_overlay(self):
        self.assertEqual(self.run_script(container=True).returncode, 0)
        active = self.root / '.codex2api-upgrade/x-x-codex2api/codex2api/active-image.json'
        self.state['compose_file'] = str(self.compose) + ',' + str(active)
        self.state['calls'] = []
        self.state['new_image'] = 'sha256:' + 'c' * 64
        self.save()
        result = self.run_script(container=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        command = self.up_calls()[0]
        files = [command[i+1] for i, part in enumerate(command[:-1]) if part == '-f']
        self.assertEqual(files, [str(self.compose), str(active)])

    def test_pull_failure_keeps_existing_container(self):
        self.state['pull_fail'] = True
        self.save()
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.state['current_image'], OLD)
        self.assertEqual(self.up_calls(), [])
        self.assertFalse(any(call[0] == 'tag' for call in self.state['calls']))

    def test_rollback_uses_saved_local_image_without_pull(self):
        self.assertEqual(self.run_script().returncode, 0)
        self.state['calls'] = []
        self.save()
        result = self.run_script('--rollback')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.state['current_image'], OLD)
        self.assertFalse(any('pull' in call for call in self.state['calls']))

    def test_dry_run_is_read_only(self):
        result = self.run_script('--dry-run')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / '.codex2api-upgrade').exists())
        self.assertFalse(any(call[0] == 'tag' or 'up' in call or 'pull' in call for call in self.state['calls']))

    def test_same_image_skips_recreation(self):
        self.state['new_image'] = OLD
        self.save()
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.up_calls(), [])

    def test_failed_health_is_not_success_and_preserves_rollback(self):
        self.state['unhealthy'] = True
        self.save()
        result = self.run_script('--timeout', '1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('健康检查未通过', result.stderr)
        self.assertTrue((self.root / '.codex2api-upgrade/x-x-codex2api/codex2api/previous-image').exists())

    def test_failed_recreation_retains_previous_image(self):
        self.state['up_fail'] = True
        self.save()
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.state['current_image'], OLD)
        self.assertTrue((self.root / '.codex2api-upgrade/x-x-codex2api/codex2api/previous-image').exists())

    def test_health_of_an_old_image_does_not_pass_upgrade(self):
        self.state['wrong_image'] = True
        self.save()
        result = self.run_script('--timeout', '1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('健康检查未通过', result.stderr)

    def test_production_wrapper_preserves_the_named_project(self):
        env = {**self.env, 'CORE_COMPOSE_FILE': str(self.compose)}
        result = subprocess.run(['bash', str(ROOT / 'scripts/upgrade-prod.sh')], cwd=self.root, env=env, text=True, capture_output=True, timeout=15)
        self.state = json.loads(self.state_file.read_text())
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.up_calls()[0][-1], 'codex2api')
        self.assertIn('x-x-codex2api', self.up_calls()[0])

    def test_scaled_target_is_rejected_before_pull(self):
        self.state['scaled'] = True
        self.save()
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('pull' in call or 'up' in call for call in self.state['calls']))


if __name__ == '__main__':
    unittest.main()
