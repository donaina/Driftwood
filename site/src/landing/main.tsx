import React from 'react';
import { createRoot } from 'react-dom/client';
import { Footer } from '../components/Footer';
import { Nav } from '../components/Nav';
import { ClosingCta } from './ClosingCta';
import { Facts } from './Facts';
import { Faq } from './Faq';
import { Hero } from './Hero';
import { HowItWorks } from './HowItWorks';
import { RunIt } from './RunIt';
import { Severity } from './Severity';
import { Transcript } from './Transcript';
import '../styles.css';

/* The landing page.

   The section order is the one a reader's questions arrive in, not the order
   the features were built in: what is this (hero), is it real (facts), what
   does it actually catch (severity), how (how it works), what does running it
   look like (transcript), how do I get it (run it), what about the awkward
   parts (FAQ), and then the ask again.

   There is no pricing section and no testimonial. Both are standard on a page
   shaped like this and both would be fabricated here — there are no customers
   to quote and nothing to sell. The space they would occupy is doing the work
   of the facts strip and the FAQ instead. */
const Landing: React.FC = () => (
  <>
    <Nav />
    <main>
      <Hero />
      <Facts />
      <Severity />
      <HowItWorks />
      <Transcript />
      <RunIt />
      <Faq />
      <ClosingCta />
    </main>
    <Footer />
  </>
);

const root = document.getElementById('root');
if (!root) {
  throw new Error('landing: #root is missing from index.html');
}
createRoot(root).render(
  <React.StrictMode>
    <Landing />
  </React.StrictMode>
);
