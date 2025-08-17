'use client'
import {useState} from 'react'
import {useRouter} from 'next/navigation'
import Nav from '../components/Nav'
import {apiFetch} from "../lib/api";

export default function NewChain() {
    const [chain_id, setId] = useState('')
    const [chain_name, setName] = useState('')
    const [rpc_endpoints, setRPC] = useState('http://localhost:50002')
    const [err, setErr] = useState('')
    const router = useRouter()

    const submit = async (e: any) => {
        e.preventDefault()
        setErr('')
        const res = await apiFetch('/api/chains', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                chain_id,
                chain_name,
                rpc_endpoints: rpc_endpoints.split(',').map((s: string) => s.trim())
            })
        })
        if (res.ok) router.push('/chains')
        else setErr('Failed to create chain')
    }

    return (
        <div className="container">
            <Nav/>
            <div className="card max-w-2xl">
                <h3 className="text-lg font-semibold">New Chain</h3>
                <form onSubmit={submit} className="mt-3 space-y-3">
                    <div>
                        <label className="block text-sm">Chain ID</label>
                        <input className="input" value={chain_id} onChange={e => setId(e.target.value)}/>
                    </div>
                    <div>
                        <label className="block text-sm">Chain Name</label>
                        <input className="input" value={chain_name} onChange={e => setName(e.target.value)}/>
                    </div>
                    <div>
                        <label className="block text-sm">RPC Endpoints (comma separated)</label>
                        <input className="input" value={rpc_endpoints} onChange={e => setRPC(e.target.value)}/>
                    </div>
                    <button className="btn" type="submit">Create</button>
                    {err && <p className="text-rose-400">{err}</p>}
                </form>
            </div>
        </div>
    )
}
