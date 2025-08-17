'use client'
import { useState } from 'react'
import { useRouter } from 'next/navigation'
import {apiFetch} from "./lib/api";

export default function Login() {
  const [username, setUser] = useState('admin')
  const [password, setPass] = useState('')
  const [err, setErr] = useState('')
  const router = useRouter()

  const submit = async (e: any) => {
    e.preventDefault()
    setErr('')
    const res = await apiFetch('/auth/login', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({ username, password })
    })
    if (res.ok) router.push('/chains')
    else { const j = await res.json().catch(()=>({error:'Login failed'})); setErr(j.error || 'Login failed') }
  }

  return (
    <div className="container">
      <div className="card max-w-md mx-auto mt-24">
        <h2 className="text-2xl font-bold">CanopyX Admin</h2>
        <p className="badge mt-2 inline-block">Sign in</p>
        <form onSubmit={submit} className="mt-4 space-y-3">
          <div>
            <label className="block text-sm mb-1">Username</label>
            <input className="input" value={username} onChange={e=>setUser(e.target.value)} placeholder="admin" />
          </div>
          <div>
            <label className="block text-sm mb-1">Password</label>
            <input type="password" className="input" value={password} onChange={e=>setPass(e.target.value)} placeholder="••••••••" />
          </div>
          <button className="btn" type="submit">Login</button>
        </form>
        {err && <p className="text-rose-400 mt-3">{err}</p>}
      </div>
    </div>
  )
}
