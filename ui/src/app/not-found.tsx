import Link from 'next/link'

export default function NotFound() {
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
      <h1 style={{ fontSize: '4rem', marginBottom: '0.5rem', color: 'var(--accent)' }}>404</h1>
      <h2 style={{ fontSize: '2rem', marginBottom: '1rem' }}>Page not found</h2>
      <p style={{ color: 'var(--text-secondary)', marginBottom: '2rem' }}>
        The page you are looking for does not exist.
      </p>
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
        Go home
      </Link>
    </div>
  )
}
