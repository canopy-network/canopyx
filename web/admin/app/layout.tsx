import './globals.css'
import {ToastProvider} from './components/ToastProvider'

export const metadata = {
  title: 'CanopyX Admin',
  description: 'Admin console for CanopyX indexer',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  )
}
