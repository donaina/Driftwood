import React, { useState } from 'react';
import { Button, Panel, PanelTitle, ViewHeader } from './ui';

/* Why this view no longer looks like five install commands.

   It used to be five cards, each headed by an install line for a package that
   does not exist: `npm install -g @donaina/driftwood` on two of them,
   `pip install driftwood-proxy` on two more and `gem install driftwood-proxy`
   on the last. Nothing was ever published under any of those names. The code
   under each was framework boilerplate containing no Driftwood code at all —
   the Express card's "Driftwood middleware" was a no-op whose own comment said
   so ("In a real implementation, this would send requests to Driftwood proxy /
   For now, we'll just pass through") — and the steps contradicted each other:
   install the proxy, start it against http://localhost:3000, then run your app
   on port 3000, which is the address it had just been told to proxy.

   A second, older copy of the same fiction lived in web/index.html with five
   *different* invented names (@donaina/driftwood-proxy, driftwood-django,
   driftwood-rails, DriftwoodModule.forRoot, Driftwood::Rails::Middleware). It
   was unreachable by construction and has been deleted with this rewrite.

   Driftwood is a reverse proxy: it sits in front of the API and watches what
   passes through. So integrating it is one URL — the caller points at
   Driftwood's port instead of the API's — and nothing is installed into the
   framework, no middleware is added, and the application's own code is
   untouched. That is what these cards say. The only thing that genuinely
   differs between frameworks is the dev command and the port the API listens
   on, so those are the only things that differ between the cards.

   Two smaller corrections. The filter row is now derived from the cards below
   rather than hand-listed beside them, so it cannot offer a framework that has
   no card; the old empty state was unreachable for exactly the opposite reason
   (the filter ids were the card ids to the letter), so it is gone rather than
   left as a branch no input can reach. And the header's "Show All" button went
   with it — the filter row's own "All Frameworks" button does the same thing
   one line below. */

type Integration = {
  id: string;
  name: string;
  /** The framework's own dev-server command — the reader's, not ours. */
  start: string;
  /** The port that command listens on, which is what --target has to name. */
  port: number;
  /** What this stack's callers most often reach for, so the base-URL line below
      is one they can recognise. It is an example, not a requirement: whichever
      client calls the API, the change is the same one. */
  clientLabel: string;
  client: string;
};

const INTEGRATIONS: Integration[] = [
  {
    id: 'express',
    name: 'Express.js',
    start: 'node server.js',
    port: 3000,
    clientLabel: 'JavaScript (browser or Node 18+)',
    client: `const res = await fetch('http://localhost:8787/api/users');`,
  },
  {
    id: 'fastapi',
    name: 'FastAPI',
    start: 'uvicorn main:app --reload',
    port: 8000,
    clientLabel: 'Python',
    client: `res = requests.get('http://localhost:8787/api/users', timeout=5)`,
  },
  {
    id: 'nestjs',
    name: 'NestJS',
    start: 'npm run start:dev',
    port: 3000,
    clientLabel: 'JavaScript',
    client: `const { data } = await axios.get('http://localhost:8787/api/users');`,
  },
  {
    id: 'django',
    name: 'Django',
    start: 'python manage.py runserver',
    port: 8000,
    clientLabel: 'Python',
    client: `res = requests.get('http://localhost:8787/api/users', timeout=5)`,
  },
  {
    id: 'rails',
    name: 'Ruby on Rails',
    start: 'rails server',
    port: 3000,
    clientLabel: 'Ruby',
    client: `res = Net::HTTP.get_response(URI('http://localhost:8787/api/users'))`,
  },
];

/** How a card's step is rendered: a sentence, then the line to run or paste. */
const Code: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div className="bg-bg-hover rounded-lg border border-border-color p-4">
    <pre className="text-xs font-mono text-text-main whitespace-pre-wrap">{children}</pre>
  </div>
);

const IntegrationsLibrary: React.FC = () => {
  const [activeFilter, setActiveFilter] = useState('all');

  /* Derived, never hand-listed beside the data — that is what makes the empty
     case impossible rather than merely unlikely. */
  const filters = [
    { id: 'all', label: 'All Frameworks' },
    ...INTEGRATIONS.map((i) => ({ id: i.id, label: i.name })),
  ];

  const visible =
    activeFilter === 'all' ? INTEGRATIONS : INTEGRATIONS.filter((i) => i.id === activeFilter);

  return (
    <div className="space-y-6">
      <ViewHeader title="Integration Guides" />

      <Panel>
        <PanelTitle className="mb-4">How it works</PanelTitle>
        {/* The one fact the old cards got wrong five different ways: there is
            nothing to install into the framework. */}
        <p className="text-text-muted mb-4">
          Driftwood is a reverse proxy. It listens on its own port, forwards every request to your
          API, and records the JSON schemas that pass through it. Integrating it means pointing your
          caller at Driftwood&rsquo;s port instead of your API&rsquo;s — nothing is installed into your
          framework, no middleware is added, and your application&rsquo;s code does not change.
        </p>
        <Code>{`# 1. Run your API the way you already do
# 2. Put Driftwood in front of it
./drift --target http://localhost:3000

# 3. Point your caller at http://localhost:8787 instead`}</Code>
        <p className="mt-4 text-sm text-text-muted">
          The dashboard is at{' '}
          <code className="font-mono">http://localhost:8787/_driftwood/</code>. That prefix belongs
          to Driftwood and is never proxied; everything else goes to your API. Your API&rsquo;s port is
          the only thing <code className="font-mono">--target</code> needs to know.
        </p>
        {/* Stated here rather than implied, because this is the page a reader
            copies commands from and `npm install -g` would silently install
            nothing today. It is the README's own warning, in the place it
            matters most. */}
        <p className="mt-2 text-sm text-text-muted">
          <strong className="text-text-main">Not on the npm registry yet.</strong>{' '}
          <code className="font-mono">@donaina/driftwood</code> has never been published, so{' '}
          <code className="font-mono">./drift</code> above is the binary built by{' '}
          <code className="font-mono">make build</code> — see the README&rsquo;s Quick Start.{' '}
          <code className="font-mono">npx</code> and{' '}
          <code className="font-mono">npm install -g</code> are the intended interface and not a
          working one.
        </p>
        <p className="mt-2 text-sm text-text-muted">
          No API to hand? Driftwood serves a built-in contract simulator — the README&rsquo;s two-minute
          walkthrough drives a real breaking change through the real diff engine with nothing
          installed and nothing behind the target.
        </p>
      </Panel>

      <Panel>
        <div className="flex flex-wrap gap-2">
          {filters.map((f) => (
            <Button
              key={f.id}
              size="md"
              variant={activeFilter === f.id ? 'primary' : 'secondary'}
              aria-pressed={activeFilter === f.id}
              onClick={() => setActiveFilter(f.id)}
            >
              {f.label}
            </Button>
          ))}
        </div>
      </Panel>

      <div className="space-y-6">
        {visible.map((integration) => (
          <Panel key={integration.id}>
            <div className="mb-4">
              <h3 className="text-xl font-semibold text-text-main flex items-center gap-2">
                {/* Neutral by construction. This badge used to be drawn in
                    accent-info, which is a severity hue: the design system
                    reserves healthy/amber/red for contract state, and a product
                    name is not a contract state. */}
                <span
                  className="w-8 h-8 flex items-center justify-center rounded-lg border border-border-color bg-bg-hover text-text-muted text-lg font-semibold"
                  aria-hidden="true"
                >
                  {integration.name.charAt(0)}
                </span>
                {integration.name}
              </h3>
              <p className="mt-2 text-text-muted text-base">
                Watch a {integration.name} API&rsquo;s contracts without changing it.
              </p>
            </div>

            <div className="mb-4 space-y-4">
              <div>
                <PanelTitle level={4} size="base" className="mb-2">
                  1. Run your API
                </PanelTitle>
                <Code>{integration.start}</Code>
              </div>

              <div>
                <PanelTitle level={4} size="base" className="mb-2">
                  2. Put Driftwood in front of it
                </PanelTitle>
                <Code>{`./drift --target http://localhost:${integration.port}${
                  integration.port === 3000 ? '' : '   # your API’s port'
                }`}</Code>
              </div>

              <div>
                <PanelTitle level={4} size="base" className="mb-2">
                  3. Point your caller at Driftwood
                </PanelTitle>
                <p className="text-sm text-text-muted mb-2">
                  {integration.clientLabel} — the base URL is the whole change:
                </p>
                <Code>{integration.client}</Code>
              </div>

              <div>
                <PanelTitle level={4} size="base" className="mb-2">
                  4. Check it
                </PanelTitle>
                <p className="text-sm text-text-muted">
                  Open <code className="font-mono">http://localhost:8787/_driftwood/</code>. The
                  first response for an endpoint is recorded as its baseline; requests appear in
                  Traffic as they pass through, and a later response that breaks the baseline raises
                  an alert. Until a request has been through, there is nothing to show — the view
                  reads what the proxy actually saw, never a sample.
                </p>
              </div>
            </div>

            {/* One per card rather than once at the top, because this is the
                question each card invites. */}
            <p className="text-sm text-text-muted">
              There is no Driftwood package for {integration.name} and none is needed — the proxy
              sees your API&rsquo;s traffic without the API knowing it is there.
            </p>
          </Panel>
        ))}
      </div>
    </div>
  );
};

export default IntegrationsLibrary;
