'use client'
import { useEffect, useState, Suspense } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import Nav from '../../components/Nav'
import {apiFetch} from "../../lib/api";

function ChainDetailInner(){
  const sp = useSearchParams();
  const id = (sp.get('id')||'') as string
  const [chain,setChain]=useState<any>(null)
  const [progress,setProg]=useState<number>(0)
  const [gaps,setGaps]=useState<any[]>([])
  const [msg,setMsg]=useState('')

  useEffect(()=>{
    if(!id) return
      apiFetch(`/api/chains/${id}`, { headers:{'X-Session':'1'} }).then(r=>r.json()).then(setChain).catch(()=>{})
      apiFetch(`/api/chains/${id}/progress`, { headers:{'X-Session':'1'} }).then(r=>r.json()).then(d=>setProg(d.last||0)).catch(()=>{})
      apiFetch(`/api/chains/${id}/gaps`, { headers:{'X-Session':'1'} }).then(r=>r.json()).then(setGaps).catch(()=>{})
  },[id])

  if(!id) return <div className="container"><Nav/><p className="text-rose-400">Missing chain id.</p></div>
  if(!chain) return <div className="container"><Nav/><p>Loading…</p></div>

  // const retrigger = async (from:number,to:number)=>{
  //   setMsg('Queuing missing heights...')
  //   const heights:number[] = []
  //   for(let h=from; h<=to; h++) heights.push(h)
  //   const res = await fetch(`/admin/api/restart-heights`, {
  //     method:'POST', headers:{'Content-Type':'application/json'},
  //     body: JSON.stringify({ chain_id:id, heights })
  //   })
  //   setMsg(res.ok? 'Re-queued':'Failed')
  // }

  return (
    <div className="container">
      <Nav/>
      <div className="header">
        <h2 className="text-xl font-semibold">{chain.chain_name} <span className="badge">{chain.chain_id}</span></h2>
        <Link className="link" href="/chains">Back</Link>
      </div>
      <div className="card">
        <p><b>RPCs:</b> <span className="text-slate-300">{chain.rpc_endpoints?.join(', ')}</span></p>
        <p><b>Last indexed:</b> {progress}</p>
      </div>
      <div className="h-4" />
      <div className="card">
        <h3 className="font-semibold mb-2">Gaps</h3>
        {gaps?.length===0 && <p>No gaps detected.</p>}
        {gaps?.map((g,i)=> (
          <div key={i} className="flex items-center gap-3 mb-2">
            <code className="text-sky-300">{g.from}-{g.to}</code>
            <button className="btn" onClick={()=>() => {setMsg('NOT IMPLEMENTED YET...')}}>Re-queue</button>
          </div>
        ))}
      </div>
      {msg && <p className="mt-2">{msg}</p>}
    </div>
  )
}

export default function Page(){
  return (
    <Suspense fallback={<div className="container"><Nav/><p>Loading…</p></div>}>
      <ChainDetailInner />
    </Suspense>
  )
}
