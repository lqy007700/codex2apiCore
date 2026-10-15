# Docker 镜像发布与快速升级

推送 `main` 后，GitHub Actions 的 **Build Docker Image** 会自动构建、验证启动与版本，再发布 `linux/amd64` 和 `linux/arm64` 镜像：

- `ghcr.io/lqy007700/codex2apicore:latest`：最近一次成功发布的 main。
- `ghcr.io/lqy007700/codex2apicore:sha-<完整提交 SHA>`：对应提交的固定版本。
- 推送 `v*` 标签时另发布标签及 semver 版本；标签构建不覆盖 main 的 latest。

工作流使用本仓库的 `GITHUB_TOKEN`，不需要额外保存发布密码。构建或启动检查失败时不会执行镜像发布。发布成功后，Actions 摘要提供 digest 和提交标签。主应用 `/health` 的 `build_version` 显示构建版本，可核对是否运行新镜像。

## 首次准备

当前镜像包已验证可匿名拉取，无需登录 GHCR。若将来重新创建镜像包，GHCR 首次发布默认是 private，即使代码仓库是 public；可在镜像包页面 **Package settings → Change visibility → Public** 设置一次：

https://github.com/users/lqy007700/packages/container/codex2apicore

如需保持 private，在服务器使用有 `read:packages` 权限的个人令牌通过 `docker login ghcr.io --password-stdin` 登录；不要将令牌放到 Compose、脚本、Git 仓库或聊天中。

等待本次镜像工作流成功再升级。无需在服务器编译代码。脚本只需要 Bash、Docker 和 Compose v2；健康检查调用容器内已有的 BusyBox wget。

## 现有生产部署

以下默认值针对当前部署：

| 项目 | 值 |
| --- | --- |
| Compose 文件 | `/opt/codex2api/docker-compose.prod.yml` |
| 项目名 | `x-x-codex2api` |
| 应用服务 | `codex2api` |
| 应用容器 | `x-x-codex2api` |
| 同项目其他服务 | `postgres`、`redis` |

更新代码以取得升级脚本（此目录的 Git remote 应指向 `lqy007700/codex2apiCore`），然后执行：

```bash
cd /opt/codex2api
git pull --ff-only
bash scripts/upgrade-prod.sh
```

后续只更新运行镜像时，一条命令即可：

```bash
bash /opt/codex2api/scripts/upgrade-prod.sh
```

只读预览、不拉取和重建：

```bash
bash /opt/codex2api/scripts/upgrade-prod.sh --dry-run
```

脚本先拉取镜像，当前服务继续运行；成功后保留旧镜像、固定新镜像，仅对 `codex2api` 执行 `up -d --no-deps --no-build --pull never`，然后核对实际镜像 ID，并等待两次 `/health` 成功。不会执行 down、删除卷、清理镜像或重启 Postgres/Redis。单实例容器替换有短暂中断，正在执行的请求仍受现有停止宽限期限制。

保留原 Compose 文件、项目、端口、网络、环境和数据挂载。镜像覆盖保存在：

```text
/opt/codex2api/.codex2api-upgrade/x-x-codex2api/codex2api/active-image.json
```

**之后重建应用应继续使用升级脚本，或给原 Compose 命令追加该覆盖文件。直接使用原 Compose 文件 `up` 可能切回其中仍写着的旧镜像。** 日志、ps 等只读操作仍可使用原命令：

```bash
docker compose -p x-x-codex2api -f /opt/codex2api/docker-compose.prod.yml logs --tail=100 codex2api
```

如果部署时使用了独立 `--env-file`，升级时同样传入；脚本从容器读取到环境文件标签时会自动沿用。原先只在 shell 中临时设置的 Compose 变量，也须在升级命令环境中保留；脚本不从运行容器导出密钥。

## 固定版本和回退

升级到已发布的指定提交：

```bash
bash /opt/codex2api/scripts/upgrade-prod.sh \
  --image ghcr.io/lqy007700/codex2apicore:sha-<完整提交SHA>
```

健康检查失败会返回非零，不会假报成功，也不会自动回退数据库。需要回退应用镜像时：

```bash
bash /opt/codex2api/scripts/upgrade-prod.sh --rollback
```

此操作使用保留的旧镜像，不需要联网拉取；只切换应用镜像，不恢复数据卷、数据库或配置，执行前应确认旧程序兼容当前数据库。旧镜像应保留到新版验证完成。升级元数据与锁均在 `.codex2api-upgrade/`，不要在运行升级时删除锁目录。

## 其他部署方式

按现有 Compose 文件与服务升级：

```bash
bash scripts/upgrade.sh \
  --file /path/to/existing-compose.yml \
  --project existing-project \
  --service existing-app
```

也可按容器名读取原 Compose 的服务、项目、工作目录与配置文件：

```bash
bash scripts/upgrade.sh --container existing-container-name
```

支持多次传入 `--file` 和 `--env-file`。脚本拒绝不存在、无法确定项目或多个副本的目标。`docker run` 启动的容器没有 Compose 配置，须先保留原参数并明确迁移方案，脚本不会猜测并删除旧容器。

`compose.core.example.yml` 仅用于全新 SQLite 安装。已有 PostgreSQL/Redis 或已有 SQLite 卷的部署不要直接替换成此示例，以免切换到另一套空数据目录。

## 本地验证

```bash
bash -n scripts/upgrade.sh scripts/upgrade-prod.sh
python3 -m unittest discover -s scripts/tests -p 'test_*.py'
```

模拟测试覆盖拉取失败、应用单服务替换、项目识别、独立环境文件、固定镜像、回退、只读预览、健康失败、错误镜像、多个副本拒绝等。实际镜像构建、Compose 配置校验与镜像启动测试由 GitHub Actions 执行。
