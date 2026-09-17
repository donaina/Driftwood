import React, { useState } from 'react';
import { Button, EmptyState, Panel, PanelTitle } from './ui';

/* The filter values are the integration `id`s, so the row filters on the same
   key the data is already keyed by. The buttons carried `data-filter`
   attributes and an `active` class that was hardcoded to "All Frameworks":
   the row was designed to filter and was never wired, so five of the six
   buttons did nothing when pressed and the sixth raised an alert. */
const FILTERS = [
  { id: 'all', label: 'All Frameworks' },
  { id: 'express', label: 'Express' },
  { id: 'fastapi', label: 'FastAPI' },
  { id: 'nestjs', label: 'NestJS' },
  { id: 'django', label: 'Django' },
  { id: 'rails', label: 'Rails' },
];

const IntegrationsLibrary: React.FC = () => {
  const [activeFilter, setActiveFilter] = useState('all');
  // In a real implementation, this would fetch data from the backend or have hardcoded examples
  const integrations = [
    {
      id: 'express',
      name: 'Express.js',
      description: 'Copy-paste ready integration example for Express.js framework',
      code: `const express = require('express');
const app = express();

// Driftwood middleware to monitor API contracts
app.use((req, res, next) => {
  // In a real implementation, this would send requests to Driftwood proxy
  // For now, we'll just pass through
  next();
});

app.get('/api/users', (req, res) => {
  res.json({ id: 1, name: 'John Doe' });
});

app.listen(3000, () => {
  console.log('Server running on port 3000');
});`,
      setupSteps: [
        'Install Driftwood proxy: npm install -g @donaina/driftwood',
        'Start Driftwood pointing to your API: drift --target http://localhost:3000',
        'Run your Express app: node server.js',
        'Driftwood will now monitor traffic between your app and the API'
      ]
    },
    {
      id: 'fastapi',
      name: 'FastAPI',
      description: 'Copy-paste ready integration example for FastAPI framework',
      code: `from fastapi import FastAPI
import uvicorn

app = FastAPI()

@app.get("/api/users")
async def read_users():
    return [{"id": 1, "name": "John Doe"}]

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)`,
      setupSteps: [
        'Install Driftwood proxy: pip install driftwood-proxy',
        'Start Driftwood pointing to your API: drift --target http://localhost:8000',
        'Run your FastAPI app: uvicorn main:app --reload',
        'Driftwood will now monitor traffic between your app and the API'
      ]
    },
    {
      id: 'nestjs',
      name: 'NestJS',
      description: 'Copy-paste ready integration example for NestJS framework',
      code: `import { Controller, Get } from '@nestjs/common';

@Controller('api')
export class UsersController {
  @Get('users')
  getUsers() {
    return [{ id: 1, name: 'John Doe' }];
  }
}`,
      setupSteps: [
        'Install Driftwood proxy: npm install -g @donaina/driftwood',
        'Start Driftwood pointing to your API: drift --target http://localhost:3000',
        'Run your NestJS app: npm run start:dev',
        'Driftwood will now monitor traffic between your app and the API'
      ]
    },
    {
      id: 'django',
      name: 'Django',
      description: 'Copy-paste ready integration example for Django framework',
      code: `from django.http import JsonResponse
from django.views import View

class UserView(View):
    def get(self, request):
        return JsonResponse([{'id': 1, 'name': 'John Doe'}], safe=False)`,
      setupSteps: [
        'Install Driftwood proxy: pip install driftwood-proxy',
        'Start Driftwood pointing to your API: drift --target http://localhost:8000',
        'Run your Django app: python manage.py runserver',
        'Driftwood will now monitor traffic between your app and the API'
      ]
    },
    {
      id: 'rails',
      name: 'Ruby on Rails',
      description: 'Copy-paste ready integration example for Ruby on Rails framework',
      code: `class UsersController < ApplicationController
  def index
    render json: [{ id: 1, name: 'John Doe' }]
  end
end`,
      setupSteps: [
        'Install Driftwood proxy: gem install driftwood-proxy',
        'Start Driftwood pointing to your API: drift --target http://localhost:3000',
        'Run your Rails app: rails server',
        'Driftwood will now monitor traffic between your app and the API'
      ]
    }
  ];

  const visible =
    activeFilter === 'all'
      ? integrations
      : integrations.filter((i) => i.id === activeFilter);

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-text-main mb-4">
          Integration Guides
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Copy-paste ready integration examples for popular frameworks
        </p>
        <div className="flex justify-center mt-4">
          <Button variant="primary" onClick={() => setActiveFilter('all')}>
            Show All
          </Button>
        </div>
      </div>

      {/* Filters */}
      <Panel>
        <div className="flex flex-wrap gap-2">
          {FILTERS.map((f) => (
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

      {/* Integrations Grid */}
      <div className="space-y-6">
        {visible.length === 0 && (
          <EmptyState
            title="No guide for that framework yet"
            body="The library covers Express, FastAPI, NestJS, Django and Rails. Choose All Frameworks to see the full set."
            action={
              <Button variant="primary" onClick={() => setActiveFilter('all')}>
                Show All
              </Button>
            }
          />
        )}
        {visible.map(integration => (
          <Panel key={integration.id}>
            <div className="mb-4">
              <h3 className="text-xl font-semibold text-text-main flex items-center space-x-2">
                <div className="w-8 h-8 flex items-center justify-center rounded-lg bg-accent-info/20">
                  <span className="text-accent-info text-lg font-semibold">{integration.name.charAt(0)}</span>
                </div>
                {integration.name}
              </h3>
              <p className="mt-2 text-text-muted text-base">
                {integration.description}
              </p>
            </div>

            {/* Code Block */}
            <div className="mb-4">
              <PanelTitle level={4} size="base" className="mb-2">
                Integration Code
              </PanelTitle>
              <div className="bg-bg-hover rounded-lg border border-border-color p-4">
                <pre className="text-xs font-mono text-text-main whitespace-pre-wrap">
{integration.code}
                </pre>
              </div>
            </div>

            {/* Setup Steps */}
            <div className="mb-4">
              <PanelTitle level={4} size="base" className="mb-2">
                Setup Steps:
              </PanelTitle>
              <ol className="list-decimal list-inside space-y-1 text-sm text-text-muted">
                {integration.setupSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>
          </Panel>
        ))}
      </div>
    </div>
  );
};

export default IntegrationsLibrary;