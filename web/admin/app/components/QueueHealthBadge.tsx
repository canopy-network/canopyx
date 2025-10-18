'use client'

import { QUEUE_BACKLOG_LOW_WATERMARK, QUEUE_BACKLOG_HIGH_WATERMARK } from '../lib/constants'

type QueueHealthBadgeProps = {
  liveDepth: number
  liveAge: number
  historicalDepth: number
  historicalAge: number
  opsQueue: {
    pending_workflow: number
    backlog_age_secs: number
  }
  compact?: boolean // For use in dashboard expanded view
}

function formatAge(secs: number): string {
  if (secs < 60) return `${Math.round(secs)}s`
  if (secs < 3600) return `${Math.round(secs / 60)}m`
  return `${Math.round(secs / 3600)}h`
}

function getBacklogColor(count: number): string {
  if (count >= QUEUE_BACKLOG_HIGH_WATERMARK) return 'text-rose-400'
  if (count > QUEUE_BACKLOG_LOW_WATERMARK) return 'text-amber-400'
  return 'text-emerald-400'
}

function getBacklogBg(count: number): string {
  if (count >= QUEUE_BACKLOG_HIGH_WATERMARK) return 'bg-rose-500/10 border-rose-500/30'
  if (count > QUEUE_BACKLOG_LOW_WATERMARK) return 'bg-amber-500/10 border-amber-500/30'
  return 'bg-emerald-500/10 border-emerald-500/30'
}

function getBacklogLabel(count: number): string {
  if (count >= QUEUE_BACKLOG_HIGH_WATERMARK) return 'Critical'
  if (count > QUEUE_BACKLOG_LOW_WATERMARK) return 'Under Load'
  return 'Healthy'
}

export function QueueHealthBadge({
  liveDepth,
  liveAge,
  historicalDepth,
  historicalAge,
  opsQueue,
  compact = false,
}: QueueHealthBadgeProps) {
  const totalBacklog = liveDepth + historicalDepth + opsQueue.pending_workflow
  const backlogColor = getBacklogColor(totalBacklog)
  const backlogBg = getBacklogBg(totalBacklog)
  const backlogLabel = getBacklogLabel(totalBacklog)

  if (compact) {
    // Compact view for dashboard expanded rows
    return (
      <div className="grid grid-cols-3 gap-2">
        {/* Overall Status Badge */}
        <div className={`col-span-3 flex items-center justify-between rounded-lg border p-2 ${backlogBg}`}>
          <div className="flex items-center gap-2">
            <div className={`h-2 w-2 rounded-full ${backlogColor.replace('text-', 'bg-')}`}></div>
            <span className="text-xs font-medium text-white">{backlogLabel}</span>
          </div>
          <span className={`font-mono text-sm font-bold ${backlogColor}`}>
            {totalBacklog.toLocaleString()}
          </span>
        </div>

        {/* Live Queue */}
        <div className="rounded border border-slate-800 bg-slate-900/30 p-2">
          <div className="text-[10px] font-medium text-slate-500 uppercase">Live</div>
          <div className="mt-1 flex items-baseline gap-1">
            <span className="font-mono text-sm font-semibold text-slate-300">
              {liveDepth.toLocaleString()}
            </span>
            {liveAge > 0 && (
              <span className="text-[10px] text-slate-600">· {formatAge(liveAge)}</span>
            )}
          </div>
        </div>

        {/* Historical Queue */}
        <div className="rounded border border-slate-800 bg-slate-900/30 p-2">
          <div className="text-[10px] font-medium text-slate-500 uppercase">Historical</div>
          <div className="mt-1 flex items-baseline gap-1">
            <span className="font-mono text-sm font-semibold text-slate-300">
              {historicalDepth.toLocaleString()}
            </span>
            {historicalAge > 0 && (
              <span className="text-[10px] text-slate-600">· {formatAge(historicalAge)}</span>
            )}
          </div>
        </div>

        {/* Ops Queue */}
        <div className="rounded border border-slate-800 bg-slate-900/30 p-2">
          <div className="text-[10px] font-medium text-slate-500 uppercase">Ops</div>
          <div className="mt-1 flex items-baseline gap-1">
            <span className="font-mono text-sm font-semibold text-slate-300">
              {opsQueue.pending_workflow.toLocaleString()}
            </span>
            {opsQueue.backlog_age_secs > 0 && (
              <span className="text-[10px] text-slate-600">· {formatAge(opsQueue.backlog_age_secs)}</span>
            )}
          </div>
        </div>
      </div>
    )
  }

  // Full view for chain detail page
  return (
    <div className="space-y-3">
      {/* Overall Health Status */}
      <div className={`flex items-center justify-between rounded-lg border p-3 ${backlogBg}`}>
        <div className="flex items-center gap-3">
          <div className={`flex h-8 w-8 items-center justify-center rounded-full ${backlogColor.replace('text-', 'bg-')}/20`}>
            {totalBacklog >= QUEUE_BACKLOG_HIGH_WATERMARK ? (
              <svg className={`h-4 w-4 ${backlogColor}`} fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
              </svg>
            ) : totalBacklog > QUEUE_BACKLOG_LOW_WATERMARK ? (
              <svg className={`h-4 w-4 ${backlogColor}`} fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
              </svg>
            ) : (
              <svg className={`h-4 w-4 ${backlogColor}`} fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
              </svg>
            )}
          </div>
          <div>
            <div className="text-sm font-semibold text-white">{backlogLabel}</div>
            <div className="text-xs text-slate-400">
              {totalBacklog < QUEUE_BACKLOG_LOW_WATERMARK && `Below ${QUEUE_BACKLOG_LOW_WATERMARK} task threshold`}
              {totalBacklog >= QUEUE_BACKLOG_LOW_WATERMARK && totalBacklog < QUEUE_BACKLOG_HIGH_WATERMARK && `Between ${QUEUE_BACKLOG_LOW_WATERMARK} and ${QUEUE_BACKLOG_HIGH_WATERMARK} tasks`}
              {totalBacklog >= QUEUE_BACKLOG_HIGH_WATERMARK && `Above ${QUEUE_BACKLOG_HIGH_WATERMARK} task threshold`}
            </div>
          </div>
        </div>
        <div className="text-right">
          <div className={`font-mono text-2xl font-bold ${backlogColor}`}>
            {totalBacklog.toLocaleString()}
          </div>
          <div className="text-xs text-slate-500">total tasks</div>
        </div>
      </div>

      {/* Queue Breakdown - Order: Live, Historical, Ops */}
      <div className="grid gap-3 md:grid-cols-3">
        {/* Live Queue */}
        <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
          <div className="flex items-center justify-between">
            <div className="text-xs font-medium text-slate-400">Live Queue</div>
            <span className="badge-neutral text-[10px]">live:*</span>
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="font-mono text-xl font-bold text-slate-200">
              {liveDepth.toLocaleString()}
            </span>
            <span className="text-xs text-slate-500">tasks</span>
          </div>
          {liveAge > 0 && (
            <div className="mt-1 text-xs text-slate-600">
              Age: {formatAge(liveAge)}
            </div>
          )}
        </div>

        {/* Historical Queue */}
        <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
          <div className="flex items-center justify-between">
            <div className="text-xs font-medium text-slate-400">Historical Queue</div>
            <span className="badge-neutral text-[10px]">historical:*</span>
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="font-mono text-xl font-bold text-slate-200">
              {historicalDepth.toLocaleString()}
            </span>
            <span className="text-xs text-slate-500">tasks</span>
          </div>
          {historicalAge > 0 && (
            <div className="mt-1 text-xs text-slate-600">
              Age: {formatAge(historicalAge)}
            </div>
          )}
        </div>

        {/* Ops Queue */}
        <div className="rounded-lg border border-slate-800 bg-slate-900/30 p-3">
          <div className="flex items-center justify-between">
            <div className="text-xs font-medium text-slate-400">Ops Queue</div>
            <span className="badge-neutral text-[10px]">ops:*</span>
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="font-mono text-xl font-bold text-slate-200">
              {opsQueue.pending_workflow.toLocaleString()}
            </span>
            <span className="text-xs text-slate-500">tasks</span>
          </div>
          {opsQueue.backlog_age_secs > 0 && (
            <div className="mt-1 text-xs text-slate-600">
              Age: {formatAge(opsQueue.backlog_age_secs)}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}