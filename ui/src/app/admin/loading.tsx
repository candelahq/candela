export default function AdminLoading() {
  return (
    <div
      role="status"
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100vh',
        backgroundColor: 'var(--bg-primary)'
      }}
    >
      <style dangerouslySetInnerHTML={{
        __html: `
          @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
          }
        `
      }} />
      <div
        aria-hidden="true"
        style={{
          width: '40px',
          height: '40px',
          border: '4px solid var(--bg-tertiary)',
          borderTop: '4px solid var(--accent)',
          borderRadius: '50%',
          animation: 'spin 1s linear infinite'
        }}
      />
      <span style={{
        position: 'absolute',
        width: '1px',
        height: '1px',
        padding: 0,
        margin: '-1px',
        overflow: 'hidden',
        clip: 'rect(0, 0, 0, 0)',
        whiteSpace: 'nowrap',
        borderWidth: 0
      }}>
        Loading…
      </span>
    </div>
  )
}
