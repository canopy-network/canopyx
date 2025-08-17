import './globals.css'

export const metadata = {
  title: 'CanopyX Admin',
  description: 'Admin console for CanopyX indexer',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}
