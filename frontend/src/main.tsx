import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter} from 'react-router-dom'
import App from './core/App'
import './core/styles.css'
const root=document.getElementById('root')
if(!root)throw new Error('未找到 root 节点')
createRoot(root).render(<StrictMode><BrowserRouter basename="/admin"><App/></BrowserRouter></StrictMode>)
