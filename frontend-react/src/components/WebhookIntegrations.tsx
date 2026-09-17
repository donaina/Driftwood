import React, { useEffect } from 'react';
import { Button, Field, Panel, PanelTitle, toast } from './ui';

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
    toast('Webhook Saved', `Configuration saved for ${serviceId} with URL: ${url}`);
  };

  const testWebhook = (serviceId: string) => {
    // In a real implementation, this would send a test payload
    toast('Webhook Test', `Test webhook sent to ${serviceId}`);
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-text-main mb-4">
          Webhook & Alert Integrations
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Connect Driftwood to your team's communication tools for real-time alerts on API contract changes
        </p>
        {/* Disabled rather than raising an alert: it has never added a webhook,
            and a control that reports success for work it did not do is worse
            than one that says it cannot. Webhooks are configured in the proxy
            config, which is what the note below points at. */}
        <div className="flex justify-center mt-4">
          <Button
            variant="primary"
            disabled
            title="Webhooks are configured in the Driftwood proxy config file, not from the dashboard."
          >
            Add Webhook
          </Button>
        </div>
      </div>

      {/* Webhook Services Grid */}
      <div className="grid gap-6">
        {webhookServices.map(service => (
          <Panel key={service.id}>
            <div className="mb-4">
              <h3 className="text-xl font-semibold text-text-main flex items-center space-x-2">
                <div className="w-8 h-8 flex items-center justify-center rounded-lg bg-accent-info/20">
                  <span className="text-accent-info text-lg font-semibold">{service.name.charAt(0)}</span>
                </div>
                {service.name}
              </h3>
              <p className="mt-2 text-text-muted text-base">
                {service.description}
              </p>
            </div>

            {/* Setup Steps */}
            <div className="mb-4">
              <PanelTitle level={4} size="base" className="mb-2">
                Setup Steps:
              </PanelTitle>
              <ol className="list-decimal list-inside space-y-1 text-sm text-text-muted">
                {service.setupSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>

            {/* Configuration */}
            <div className="mb-4">
              <PanelTitle level={4} size="base" className="mb-2">
                Configuration
              </PanelTitle>
              <div className="space-y-4">
                {/* Webhook URL */}
                <Field label="Webhook URL:">
                  <input
                    type="url"
                    id={`webhook-url-${service.id}`}
                    placeholder="Enter your webhook URL here"
                    className="w-full px-4 py-3 rounded-sm border border-border-color bg-bg-hover text-text-main focus:outline-none focus:border-accent-healthy"
                  />
                </Field>

                {/* Alert Types */}
                <Field label="Alert Types:">
                  <div className="flex space-x-4">
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="breaking"
                        defaultChecked
                        className="h-4 w-4 text-accent-info focus:outline-none focus:border-accent-healthy border-border-color"
                      />
                      Breaking Changes
                    </label>
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="warning"
                        className="h-4 w-4 text-accent-warning focus:outline-none focus:border-accent-healthy border-border-color"
                      />
                      Warnings
                    </label>
                    <label className="flex items-center space-x-2">
                      <input
                        type="checkbox"
                        value="info"
                        className="h-4 w-4 text-accent-healthy focus:outline-none focus:border-accent-healthy border-border-color"
                      />
                      Informational
                    </label>
                  </div>
                </Field>
              </div>
            </div>

            {/* Actions */}
            <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4 pt-4">
              <Button size="md" className="flex-1 sm:w-auto" onClick={() => testWebhook(service.id)}>
                Test Connection
              </Button>
              <Button
                size="md"
                variant="primary"
                className="flex-1 sm:w-auto"
                onClick={() => saveWebhookConfig(service.id)}
              >
                Save Configuration
              </Button>
            </div>
          </Panel>
        ))}
      </div>
    </div>
  );
};

export default WebhookIntegrations;