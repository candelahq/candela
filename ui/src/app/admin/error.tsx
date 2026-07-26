'use client'

import { useEffect } from 'react'
import Link from 'next/link'

export default function AdminError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error('[candela:admin] Unhandled error:', error)
  }, [error])

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
        <h2 style={{ color: 'var(--text-primary)', marginBottom: '1rem' }}>Admin error</h2>
        <p style={{ color: 'var(--text-secondary)', marginBottom: '1.5rem' }}>
          An unexpected error occurred in the admin panel. Please try again.
        </p>
        {error.digest && (
          <p style={{
            color: 'var(--text-muted)',
            fontSize: '12px',
            marginBottom: '1.5rem'
          }}>
            Error ID: {error.digest}
          </p>
        )}
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
            href="/admin"
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
            Return to admin
          </Link>
        </div>
      </div>
    </div>
  )
}
