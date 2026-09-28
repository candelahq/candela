"use client";

import Link from 'next/link';
import { usePageTitle } from '@/hooks/usePageTitle';

export default function TraceNotFound() {
  usePageTitle("Trace Not Found");
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
        The trace you&apos;re looking for doesn&apos;t exist or has been deleted.
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
