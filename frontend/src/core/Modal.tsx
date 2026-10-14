import {useEffect, useRef, type ReactNode} from 'react'
import {X} from 'lucide-react'

export function Modal({title,children,onClose}:{title:string;children:ReactNode;onClose:()=>void}) {
 const box=useRef<HTMLDivElement>(null), close=useRef(onClose)
 close.current=onClose
 useEffect(()=>{
  const previous=document.activeElement as HTMLElement|null
  const bodyOverflow=document.body.style.overflow;document.body.style.overflow='hidden'
  box.current?.focus()
  const key=(event:KeyboardEvent)=>{
   if(event.key==='Escape')close.current()
   if(event.key==='Tab'){
    const elements=box.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href]')
    if(!elements?.length)return
    if(event.shiftKey && (document.activeElement===elements[0] || document.activeElement===box.current)){event.preventDefault();elements[elements.length-1].focus()}
    else if(!event.shiftKey && document.activeElement===elements[elements.length-1]){event.preventDefault();elements[0].focus()}
   }
  }
  document.addEventListener('keydown',key)
  return()=>{document.body.style.overflow=bodyOverflow;document.removeEventListener('keydown',key);previous?.focus()}
 },[])
 return <div className="modal-backdrop"><div className="modal" role="dialog" aria-modal="true" aria-label={title} ref={box} tabIndex={-1}><div className="modal-heading"><h2>{title}</h2><button aria-label="关闭" onClick={onClose}><X size={18}/></button></div>{children}</div></div>
}
