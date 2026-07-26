'use client'

import Link from 'next/link'

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      backgroundColor: 'var(--bg-primary)'
    }}>
      <div style={{
        textAlign: 'center',
        maxWidth: '500px',
        padding: '2rem',
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-lg)'
      }}>
        <div style={{ fontSize: '3rem', marginBottom: '1rem' }}>🕯️</div>
        <h2 style={{ color: 'var(--text-primary)', marginBottom: '1rem' }}>Something went wrong</h2>
        <pre style={{
          backgroundColor: 'var(--bg-tertiary)',
          color: 'var(--error)',
          padding: '1rem',
          borderRadius: 'var(--radius-md)',
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
              backgroundColor: 'var(--accent)',
              color: 'var(--bg-primary)',
              border: 'none',
              padding: '0.5rem 1rem',
              borderRadius: 'var(--radius-md)',
              cursor: 'pointer',
              fontWeight: 'bold'
            }}
          >
            Try again
          </button>
          <Link 
            href="/"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--text-primary)',
              border: '1px solid var(--border)',
              padding: '0.5rem 1rem',
              borderRadius: 'var(--radius-md)',
              cursor: 'pointer',
              textDecoration: 'none'
            }}
          >
            Return home
          </Link>
        </div>
      </div>
    </div>
  )
}
