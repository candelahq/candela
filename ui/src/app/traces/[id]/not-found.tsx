import Link from 'next/link'

export default function TraceNotFound() {
  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '100vh',
      backgroundColor: 'var(--bg-primary)',
      color: 'var(--text-primary)'
    }}>
      <h2 style={{ fontSize: '2rem', marginBottom: '1rem' }}>Trace not found</h2>
      <p style={{ color: 'var(--text-secondary)', marginBottom: '2rem' }}>
        The trace you're looking for doesn't exist or has been deleted.
      </p>
      <Link 
        href="/traces"
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
        Back to traces
      </Link>
    </div>
  )
}
