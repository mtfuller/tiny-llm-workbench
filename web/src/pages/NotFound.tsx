import { Link, useLocation } from 'react-router-dom'

function NotFound() {
  const location = useLocation()

  return (
    <>
      <div className="page-header">
        <h2>Page not found</h2>
      </div>
      <div className="empty-state">
        <p>
          Nothing lives at <code>{location.pathname}</code>.
        </p>
        <p style={{ marginTop: '0.75rem' }}>
          <Link to="/">Go back home</Link>
        </p>
      </div>
    </>
  )
}

export default NotFound
