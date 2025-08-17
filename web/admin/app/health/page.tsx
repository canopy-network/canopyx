'use client'
import { useEffect, useState } from 'react'
import Nav from '../components/Nav'
import {apiFetch} from "../lib/api";

export default function Health(){
  const [data,setData]=useState<any>(null)
  useEffect(()=>{
      apiFetch('/api/health', { headers:{'X-Session':'1'} })
      .then(r=>r.json()).then(setData).catch(()=>setData({error:'unavailable'}))
  },[])
  return (
    <div className="container">
      <Nav/>
      {!data && <p>Loading…</p>}
      {data && (
        <div className="space-y-4">
          <div className="card">
            <h3 className="font-semibold mb-2">Temporal</h3>
            <pre className="text-slate-300 text-sm overflow-auto">{JSON.stringify(data.temporal, null, 2)}</pre>
          </div>
          <div className="card">
            <h3 className="font-semibold mb-2">Schedules</h3>
            <pre className="text-slate-300 text-sm overflow-auto">{JSON.stringify(data.schedules, null, 2)}</pre>
          </div>
        </div>
      )}
    </div>
  )
}
