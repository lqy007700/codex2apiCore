import type {ReactNode} from 'react'

export function PageHeader({title,description,children}:{title:string;description:string;children?:ReactNode}) {
 return <header className="page-header"><div className="page-heading"><p className="eyebrow">工作空间</p><h1>{title}</h1><p>{description}</p></div><div className="actions">{children}</div></header>
}
