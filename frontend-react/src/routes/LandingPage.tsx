import React from 'react';
import { Link } from 'react-router-dom';

const LandingPage: React.FC = () => {
  return (
    <>
      {/* Hero Section - Inspired by Zova */}
      <section className="relative bg-bg-main min-h-[calc(100vh-4rem)] flex items-center justify-center py-12 px-4 sm:py-20 lg:py-24">
        <div className="max-w-4xl text-center">
          <div className="inline-block mb-6">
            <span className="bg-accent-info/20 rounded-full w-12 h-12 flex items-center justify-center">
              <span className="text-accent-info text-2xl font-bold">D</span>
            </span>
          </div>
          <h1 className="text-4xl font-bold text-text-main md:text-5xl lg:text-6xl mb-6 leading-tight">
            Driftwood – Real-Time API Contract Drift Detector
          </h1>
          <p className="text-xl text-text-muted mb-8 max-w-2xl mx-auto">
            Continuously monitor your API contracts, detect breaking changes early,
            and keep your services reliable.
          </p>
          <div className="flex flex-col sm:flex-row sm:justify-center sm:space-x-4">
            <Link to="/app" className="bg-accent-info text-white px-6 py-3 rounded-lg font-medium hover:bg-accent-info/90 transition-colors flex-1 sm:flex-1 sm:max-w-xs">
              Try Driftwood – Live Demo
            </Link>
            <Link to="/app" className="border border-border-color text-text-main px-6 py-3 rounded-lg font-medium hover:bg-bg-hover transition-colors flex-1 sm:flex-1 sm:max-w-xs">
              View Features
            </Link>
          </div>
        </div>
      </section>

      {/* Three-Step Onboarding Flow - Inspired by Zova */}
      <section className="bg-bg-card py-16">
        <div className="max-w-4xl mx-auto px-4">
          <h2 className="text-3xl font-semibold text-text-main mb-12 text-center">
            Get Started in 3 Simple Steps
          </h2>
          <div className="grid gap-8 sm:grid-cols-3 items-start">
            {/* Step 1 */}
            <div className="flex flex-col items-center space-y-6 text-center p-8 bg-bg-hover rounded-lg">
              <div className="w-14 h-14 bg-accent-info/20 rounded-full flex items-center justify-center mb-4">
                <span className="text-accent-info text-3xl font-bold">01</span>
              </div>
              <h3 className="text-xl font-semibold text-text-main">Connect Your API</h3>
              <p className="text-text-muted max-w-md">
                Point Driftwood at your API backend to begin monitoring contract stability.
              </p>
            </div>

            {/* Step 2 */}
            <div className="flex flex-col items-center space-y-6 text-center p-8 bg-bg-hover rounded-lg">
              <div className="w-14 h-14 bg-accent-warning/20 rounded-full flex items-center justify-center mb-4">
                <span className="text-accent-warning text-3xl font-bold">02</span>
              </div>
              <h3 className="text-xl font-semibold text-text-main">Detect Endpoints</h3>
              <p className="text-text-muted max-w-md">
                Let Driftwood automatically discover and learn your API's contract from live traffic.
              </p>
            </div>

            {/* Step 3 */}
            <div className="flex flex-col items-center space-y-6 text-center p-8 bg-bg-hover rounded-lg">
              <div className="w-14 h-14 bg-accent-healthy/20 rounded-full flex items-center justify-center mb-4">
                <span className="text-accent-healthy text-3xl font-bold">03</span>
              </div>
              <h3 className="text-xl font-semibold text-text-main">Set Baselines</h3>
              <p className="text-text-muted max-w-md">
                Establish your baseline contract and get alerted to any breaking changes.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Features Section - Icon Cards like Zova */}
      <section className="py-16">
        <div className="max-w-5xl mx-auto px-4">
          <h2 className="text-3xl font-semibold text-text-main mb-12 text-center">
            Why Teams Choose Driftwood
          </h2>
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            {/* Feature 1: Live Contract Monitoring */}
            <div className="flex flex-col items-center space-y-4 p-6 bg-bg-hover rounded-lg">
              <div className="w-12 h-12 bg-accent-info/20 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-accent-info" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 8c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 12c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0-6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z"></path>
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-text-main">Live Contract Monitoring</h3>
              <p className="text-text-muted text-center max-w-sm">
                Observe every request and compare it against your baseline contract in real time.
              </p>
            </div>

            {/* Feature 2: Instant Alerting */}
            <div className="flex flex-col items-center space-y-4 p-6 bg-bg-hover rounded-lg">
              <div className="w-12 h-12 bg-accent-warning/20 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-accent-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-text-main">Instant Alerting</h3>
              <p className="text-text-muted text-center max-w-sm">
                Get notified via webhooks (Slack, Teams, Discord, Email) the moment drift is detected.
              </p>
            </div>

            {/* Feature 3: Scenario Library */}
            <div className="flex flex-col items-center space-y-4 p-6 bg-bg-hover rounded-lg">
              <div className="w-12 h-12 bg-accent-healthy/20 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-accent-healthy" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12l2 2 4-4M7 20h10a2 2 0 002-2V6a2 2 0 00-2-2H7a2 2 0 00-2 2v12a2 2 0 002 2z"></path>
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-text-main">Scenario Library</h3>
              <p className="text-text-muted text-center max-w-sm">
                Learn about common API breaking changes with pre‑configured scenarios for experimentation.
              </p>
            </div>

            {/* Feature 4: Export & Reporting */}
            <div className="flex flex-col items-center space-y-4 p-6 bg-bg-hover rounded-lg">
              <div className="w-12 h-12 bg-accent-breaking/20 rounded-full flex items-center justify-center">
                <svg className="w-6 h-6 text-accent-breaking" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12h6m-6-4h6m2 2a2 2 0 100-4 2 2 0 000 4z"></path>
                </svg>
              </div>
              <h3 className="text-lg font-semibold text-text-main">Export & Reporting</h3>
              <p className="text-text-muted text-center max-w-sm">
                Generate shareable PDF/PNG reports and schedule automatic email updates for stakeholders.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Testimonial Section */}
      <section className="bg-bg-hover py-16">
        <div className="max-w-4xl mx-auto px-4 text-center">
          <h2 className="text-3xl font-semibold text-text-main mb-8">
            What Teams Are Saying
          </h2>
          <div className="max-w-2xl mx-auto">
            <p className="text-text-muted text-lg italic mb-6">
              "Driftwood has saved us countless hours of debugging by catching breaking changes before they reach production. The real-time alerts and clear diff views make contract monitoring effortless."
            </p>
            <div className="flex flex-col items-center space-x-4">
              <div className="w-10 h-10 bg-bg-card rounded-full flex items-center justify-center">
                <span className="text-accent-info">JD</span>
              </div>
              <div className="space-y-1 text-left">
                <p className="font-semibold text-text-main">John Doe</p>
                <p className="text-text-sm text-text-muted">Platform Engineer at TechCorp</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Call to Action Section */}
      <section className="py-16">
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
            <a href="#" className="hover:text-text-main transition-colors">
              Blog
            </a>
            <a href="#" className="hover:text-text-main transition-colors">
              Support
            </a>
          </div>
        </div>
      </footer>
    </>
  );
};

export default LandingPage;