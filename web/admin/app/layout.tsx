import './globals.css'
import { ToastProvider } from './components/ToastProvider'
import { AuthProvider } from './lib/auth-context'

export const metadata = {
  title: 'CanopyX Admin',
  description: 'Admin console for CanopyX indexer',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <AuthProvider>
          <ToastProvider>{children}</ToastProvider>
        </AuthProvider>
      </body>
    </html>
  )
}
