import React, { useEffect } from 'react';

const webhookServices = [
  {
    id: 'slack',
    name: 'Slack',
    icon: '#',
    description: 'Send drift alerts to Slack channels for team notification',
    setupSteps: [
      'Create a Slack app and enable Incoming Webhooks',
      'Copy the webhook URL from Slack',
      'Paste the URL below and save',
      'Configure alert types and frequency'
    ]
  },
  {
    id: 'teams',
    name: 'Microsoft Teams',
    icon: '#',
    description: 'Send drift alerts to Microsoft Teams channels',
    setupSteps: [
      'Configure an Incoming Webhook connector in Teams',
      'Copy the webhook URL from Teams',
      'Paste the URL below and save',
      'Configure alert types and frequency'
    ]
  },
  {
    id: 'discord',
    name: 'Discord',
    icon: '#',
    description: 'Send drift alerts to Discord channels',
    setupSteps: [
      'Create a webhook in your Discord channel settings',
      'Copy the webhook URL from Discord',
      'Paste the URL below and save',
      'Configure alert types and frequency'
    ]
  },
  {
    id: 'email',
    name: 'Email',
    icon: '#',
    description: 'Send drift alerts via email',
    setupSteps: [
      'Configure SMTP settings in the backend',
      'Add recipient email addresses',
      'Set email template preferences',
      'Configure alert types and frequency'
    ]
  }
];

const WebhookIntegrations: React.FC = () => {
  // Simulate the showWebhook event to ensure component is ready when shown
  useEffect(() => {
    const handleShowWebhook = () => {
      // Component is already rendered; we just ensure it's visible
      const root = document.getElementById('webhook-react-root');
      if (root) {
        root.style.display = 'block';
      }
    };
    window.addEventListener('showWebhook', handleShowWebhook);
    return () => {
      window.removeEventListener('showWebhook', handleShowWebhook);
    };
  }, []);

  const saveWebhookConfig = (serviceId: string) => {
    const urlInput = document.getElementById(`webhook-url-${serviceId}`) as HTMLInputElement | null;
    const url = urlInput ? urlInput.value : '';
    // In a real implementation, this would save to backend
    alert(`Configuration saved for ${serviceId} with URL: ${url}`);
  };

  const testWebhook = (serviceId: string) => {
    // In a real implementation, this would send a test payload
    alert(`Test webhook sent to ${serviceId}`);
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Webhook & Alert Integrations
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Connect Driftwood to your team's communication tools for real-time alerts on API contract changes
        </p>
        <div className="flex justify-center mt-4">
          <button
            className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors"
            onClick={() => alert('Add new webhook configuration')}
          >
            Add Webhook
          </button>
        </div>
      </div>

      {/* Webhook Services Grid */}
      <div className="grid gap-6">
        {webhookServices.map(service => (
          <div key={service.id} className="bg-bg-card rounded-xl border border-border-color p-6">
            <div className="mb-4">
              <h3 className="text-xl font-semibold text-text-main flex items-center space-x-2">
                <div className="w-8 h-8 flex items-center justify-center rounded-lg bg-accent-info/20">
                  <span className="text-accent-info text-lg font-bold">{service.name.charAt(0)}</span>
                </div>
                {service.name}
              </h3>
              <p className="mt-2 text-text-muted text-base">
                {service.description}
              </p>
            </div>

            {/* Setup Steps */}
            <div className="mb-4">
              <h4 className="font-semibold text-text-main mb-2">
                Setup Steps:
              </h4>
              <ol className="list-decimal list-inside space-y-1 text-sm text-text-muted">
                {service.setupSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>

            {/* Configuration */}
            <div className="mb-4">
              <h4 className="font-semibold text-text-main mb-2">
                Configuration
              </h4>
              <div className="space-y-4">
                {/* Webhook URL */}
                <div>
                  <label className="block text-sm font-medium text-text-muted mb-2">
                    Webhook URL:
                  </label>
                  <input
                    type="url"
                    id={`webhook-url-${service.id}`}
                    placeholder="Enter your webhook URL here"
                    className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
                  />
                </div>

                {/* Alert Types */}
                <div>
                  <label className="block text-sm font-medium text-text-muted mb-2">
                    Alert Types:
                  </label>
                  <div className="flex space-x-4">
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="breaking"
                        defaultChecked
                        className="h-4 w-4 text-accent-info focus:ring-accent-info border-border-color"
                      />
                      Breaking Changes
                    </label>
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="warning"
                        className="h-4 w-4 text-accent-warning focus:ring-accent-warning border-border-color"
                      />
                      Warnings
                    </label>
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="info"
                        className="h-4 w-4 text-accent-healthy focus:ring-accent-healthy border-border-color"
                      />
                      Informational
                    </label>
                  </div>
                </div>
              </div>
            </div>

            {/* Actions */}
            <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4 pt-4">
              <button
                onClick={() => testWebhook(service.id)}
                className="px-4 py-2 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors flex-1 sm:auto"
              >
                Test Connection
              </button>
              <button
                onClick={() => saveWebhookConfig(service.id)}
                className="px-4 py-2 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors flex-1 sm:auto"
              >
                Save Configuration
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default WebhookIntegrations;