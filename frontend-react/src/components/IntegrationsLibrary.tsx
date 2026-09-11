import React from 'react';

const IntegrationsLibrary: React.FC = () => {
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

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Integration Guides
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Copy-paste ready integration examples for popular frameworks
        </p>
        <div className="flex justify-center mt-4">
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={() => alert('Show All')}
          >
            Show All
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <div className="flex flex-wrap gap-2">
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors active"
            data-filter="all"
          >
            All Frameworks
          </button>
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            data-filter="express"
          >
            Express
          </button>
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            data-filter="fastapi"
          >
            FastAPI
          </button>
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            data-filter="nestjs"
          >
            NestJS
          </button>
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            data-filter="django"
          >
            Django
          </button>
          <button
            className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors"
            data-filter="rails"
          >
            Rails
          </button>
        </div>
      </div>

      {/* Integrations Grid */}
      <div className="space-y-6">
        {integrations.map(integration => (
          <div key={integration.id} className="bg-bg-card rounded-xl border border-border-color p-6">
            <div className="mb-4">
              <h3 className="text-xl font-semibold text-text-main flex items-center space-x-2">
                <div className="w-8 h-8 flex items-center justify-center rounded-lg bg-accent-info/20">
                  <span className="text-accent-info text-lg font-bold">{integration.name.charAt(0)}</span>
                </div>
                {integration.name}
              </h3>
              <p className="mt-2 text-text-muted text-base">
                {integration.description}
              </p>
            </div>

            {/* Code Block */}
            <div className="mb-4">
              <h4 className="font-semibold text-text-main mb-2">
                Integration Code
              </h4>
              <div className="bg-bg-hover rounded-lg border border-border-color p-4">
                <pre className="text-xs font-mono text-text-main whitespace-pre-wrap">
{integration.code}
                </pre>
              </div>
            </div>

            {/* Setup Steps */}
            <div className="mb-4">
              <h4 className="font-semibold text-text-main mb-2">
                Setup Steps:
              </h4>
              <ol className="list-decimal list-inside space-y-1 text-sm text-text-muted">
                {integration.setupSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default IntegrationsLibrary;