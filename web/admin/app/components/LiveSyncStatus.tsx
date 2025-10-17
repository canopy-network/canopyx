type LiveSyncStatusProps = {
  last_indexed: number
  head: number
  missing_blocks_count?: number
  is_live_sync?: boolean
}

export function LiveSyncStatus({
  last_indexed,
  head,
  missing_blocks_count = 0,
  is_live_sync = false,
}: LiveSyncStatusProps) {
  const progress = head > 0 ? (last_indexed / head) * 100 : 0
  const lag = head - last_indexed

  return (
    <div className="space-y-4">
      {/* Live Sync Badge and Current Position */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          {is_live_sync ? (
            <span className="badge-success flex items-center gap-1.5">
              <svg className="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 20 20">
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                  clipRule="evenodd"
                />
              </svg>
              Live Sync
            </span>
          ) : (
            <span className="badge-warning flex items-center gap-1.5">
              <svg className="h-3.5 w-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                ></circle>
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
              Catching Up
            </span>
          )}
          <div className="text-sm text-slate-400">
            <span className="font-mono text-white">{last_indexed.toLocaleString()}</span>
            <span className="mx-1">/</span>
            <span className="font-mono text-white">{head.toLocaleString()}</span>
          </div>
        </div>

        {lag > 0 && (
          <div className="text-sm text-slate-400">
            <span className="text-amber-400">-{lag.toLocaleString()}</span> blocks
          </div>
        )}
      </div>

      {/* Progress Bar */}
      <div className="space-y-2">
        <div className="h-2 w-full overflow-hidden rounded-full bg-slate-800">
          <div
            className={`h-full transition-all duration-500 ${
              is_live_sync
                ? 'bg-gradient-to-r from-emerald-500 to-emerald-400'
                : 'bg-gradient-to-r from-indigo-500 to-purple-500'
            }`}
            style={{ width: `${Math.min(progress, 100)}%` }}
          />
        </div>
        <div className="flex items-center justify-between text-xs">
          <span className="text-slate-500">Total Progress</span>
          <span className="font-mono font-semibold text-slate-300">{progress.toFixed(2)}%</span>
        </div>
      </div>

      {/* Historical Backlog Warning */}
      {missing_blocks_count > 0 && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3">
          <div className="flex items-start gap-2">
            <svg
              className="h-5 w-5 flex-shrink-0 text-amber-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
            <div className="flex-1">
              <div className="text-sm font-medium text-amber-200">Historical Backlog</div>
              <div className="mt-1 text-xs text-amber-300">
                {missing_blocks_count.toLocaleString()} blocks missing from historical data
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}