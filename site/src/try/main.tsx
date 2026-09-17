import React from 'react';
import { createRoot } from 'react-dom/client';
import { Footer } from '../components/Footer';
import { Nav } from '../components/Nav';
import { TryItOut } from './TryItOut';
import '../styles.css';

/* The try-it-out page.
 *
 * It is a separate HTML entry rather than a route inside the landing page
 * because it is a separate thing: the landing page is read, this one is
 * operated, and it is the only page that talks to a server. Two entries also
 * keep the landing page's bundle free of the control-plane client, which is
 * dead weight on a page that never calls it.
 *
 * It shares the nav, the footer and the token file with the landing page, so
 * the two still read as one site rather than two. */
const TryPage: React.FC = () => (
  <>
    <Nav />
    <main>
      <TryItOut />
    </main>
    <Footer />
  </>
);

const root = document.getElementById('root');
if (!root) {
  throw new Error('try: #root is missing from try.html');
}
createRoot(root).render(
  <React.StrictMode>
    <TryPage />
  </React.StrictMode>
);
