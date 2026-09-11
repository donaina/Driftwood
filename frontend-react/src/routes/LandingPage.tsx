import React from 'react';
import { Link } from 'react-router-dom';

const LandingPage: React.FC = () => {
  return (
    <>
      {/* Hero Section */}
      <section className="relative bg-bg-main min-h-screen flex items-center justify-center py-12 px-4 sm:py-20 lg:py-24">
        <div className="max-w-4xl text-center">
          <h1 className="text-4xl font-bold text-text-main md:text-5xl lg:text-6xl mb-6">
            Driftwood – Real‑Time API Contract Drift Detector
          </h1>
          <p className="text-xl text-text-muted mb-8">
            Continuously monitor your API contracts, detect breaking changes early,
            and keep your services reliable.
          </p>
          <Link to="/app" className="bg-accent-info text-white px-6 py-3 rounded-lg font-medium hover:bg-accent-info/90 transition-colors">
            Try Driftwood – Live Demo
          </Link>
        </div>
      </section>

      {/* Features Section */}
      <section className="bg-bg-card py-12">
        <div className="max-w-5xl mx-auto px-4">
          <h2 className="text-3xl font-semibold text-text-main mb-10 text-center">
            Why Teams Choose Driftwood
          </h2>
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            {/* Feature 1 */}
            <div className="bg-bg-hover p-6 rounded-lg">
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0">
                  <div className="w-10 h-10 bg-accent-healthy/20 rounded-full flex items-center justify-center">
                    <span className="text-accent-healthy text-2xl">●</span>
                  </div>
                </div>
                <div>
                  <h3 className="font-medium text-text-main mb-2">Live Contract Monitoring</h3>
                  <p className="text-text-muted">
                    Observe every request and compare it against your baseline contract
                    in real time.
                  </p>
                </div>
              </div>
            </div>
            {/* Feature 2 */}
            <div className="bg-bg-hover p-6 rounded-lg">
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0">
                  <div className="w-10 h-10 bg-accent-warning/20 rounded-full flex items-center justify-center">
                    <span className="text-accent-warning text-2xl">▲</span>
                  </div>
                </div>
                <div>
                  <h3 className="font-medium text-text-main mb-2">Instant Alerting</h3>
                  <p className="text-text-muted">
                    Get notified via webhooks (Slack, Teams, Discord, Email) the moment
                    drift is detected.
                  </p>
                </div>
              </div>
            </div>
            {/* Feature 3 */}
            <div className="bg-bg-hover p-6 rounded-lg">
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0">
                  <div className="w-10 h-10 bg-accent-breaking/20 rounded-flex items-center justify-center">
                    <span className="text-accent-breaking text-2xl">■</span>
                  </div>
                </div>
                <div>
                  <h3 className="font-medium text-text-main mb-2">Scenario Library</h3>
                  <p className="text-text-muted">
                    Learn about common API breaking changes with pre‑configured
                    scenarios for experimentation.
                  </p>
                </div>
              </div>
            </div>
            {/* Feature 4 */}
            <div className="bg-bg-hover p-6 rounded-lg">
              <div className="flex items-start space-x-4">
                <div className="flex-shrink-0">
                  <div className="w-10 h-10 bg-accent-info/20 rounded-full flex items-center justify-center">
                    <span className="text-accent-info text-2xl">🔒</span>
                  </div>
                </div>
                <div>
                  <h3 className="font-medium text-text-main mb-2">Export & Reporting</h3>
                  <p className="text-text-muted">
                    Generate shareable PDF/PNG reports and schedule automatic email
                    updates for stakeholders.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Call to Action Section */}
      <section className="bg-bg-hover py-12">
        <div className="max-w-4xl mx-auto px-4 text-center">
          <h2 className="text-3xl font-semibold text-text-main mb-6">
            Ready to see Driftwood in action?
          </h2>
          <p className="text-text-muted mb-8">
            Launch the live demo and start monitoring your APIs within seconds.
          </p>
          <Link to="/app" className="bg-accent-info text-white px-6 py-3 rounded-lg font-medium hover:bg-accent-info/90 transition-colors">
            Open the Dashboard
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="bg-bg-main text-text-muted py-8">
        <div className="max-w-4xl mx-auto px-4 flex flex-col items-center gap-4">
          <p className="text-sm">&copy; {new Date().getFullYear()} Driftwood. All rights reserved.</p>
          <div className="flex space-x-6">
            <a href="https://github.com/donaina/Driftwood" className="hover:text-text-main transition-colors" target="_blank" rel="noopener noreferrer">
              GitHub
            </a>
            <a href="#" className="hover:text-text-main transition-colors">
              Documentation
            </a>
          </div>
        </div>
      </footer>
    </>
  );
};

export default LandingPage;