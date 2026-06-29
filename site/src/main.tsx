import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import { App } from './App';

// Handle GitHub Pages SPA redirect (from 404.html)
const params = new URLSearchParams(window.location.search);
const redirectPath = params.get('p');
if (redirectPath) {
  const cleanSearch = window.location.search.replace(/[?&]p=[^&]+/, '').replace(/^&/, '?');
  window.history.replaceState(null, '', '/agents' + redirectPath + cleanSearch + window.location.hash);
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
