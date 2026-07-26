'use client'

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  return (
    <html>
      <body style={{
        backgroundColor: '#0a0a0f',
        color: '#e8e8f0',
        fontFamily: 'Inter, sans-serif',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100vh',
        margin: 0
      }}>
        <div style={{
          textAlign: 'center',
          maxWidth: '500px',
          padding: '2rem',
          backgroundColor: '#12121a',
          border: '1px solid #2a2a40',
          borderRadius: '12px'
        }}>
          <h2 style={{ marginBottom: '1rem' }}>Something went wrong</h2>
          <pre style={{
            backgroundColor: '#1a1a2e',
            color: '#f87171',
            padding: '1rem',
            borderRadius: '8px',
            overflow: 'auto',
            marginBottom: '1.5rem',
            textAlign: 'left',
            fontSize: '14px'
          }}>
            {error.message}
          </pre>
          <div style={{ display: 'flex', gap: '1rem', justifyContent: 'center' }}>
            <button 
              onClick={() => reset()}
              style={{
                backgroundColor: '#f0a030',
                color: '#0a0a0f',
                border: 'none',
                padding: '0.5rem 1rem',
                borderRadius: '8px',
                cursor: 'pointer',
                fontWeight: 'bold'
              }}
            >
              Try again
            </button>
            <a 
              href="/"
              style={{
                backgroundColor: 'transparent',
                color: '#e8e8f0',
                border: '1px solid #2a2a40',
                padding: '0.5rem 1rem',
                borderRadius: '8px',
                cursor: 'pointer',
                textDecoration: 'none'
              }}
            >
              Go home
            </a>
          </div>
        </div>
      </body>
    </html>
  )
}
