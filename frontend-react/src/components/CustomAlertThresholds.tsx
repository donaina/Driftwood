import React, { useState } from 'react';

const CustomAlertThresholds: React.FC = () => {
  const [thresholds, setThresholds] = useState({
    typeChange: 'breaking',
    requiredOptional: 'warning',
    addedRemoved: 'info',
    statusCode: 'warning',
    headerChanges: 'info'
  });

  const [preset, setPreset] = useState('recommended');

  const handlePresetChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const selectedPreset = e.target.value;
    setPreset(selectedPreset);

    // Apply preset values
    switch (selectedPreset) {
      case 'strict':
        setThresholds({
          typeChange: 'breaking',
          requiredOptional: 'breaking',
          addedRemoved: 'breaking',
          statusCode: 'breaking',
          headerChanges: 'breaking'
        });
        break;
      case 'recommended':
        setThresholds({
          typeChange: 'breaking',
          requiredOptional: 'warning',
          addedRemoved: 'info',
          statusCode: 'warning',
          headerChanges: 'info'
        });
        break;
      case 'lenient':
        setThresholds({
          typeChange: 'info',
          requiredOptional: 'info',
          addedRemoved: 'info',
          statusCode: 'info',
          headerChanges: 'info'
        });
        break;
      default:
        break;
    }
  };

  const handleThresholdChange = (e: React.ChangeEvent<HTMLSelectElement>, field: keyof typeof thresholds) => {
    setThresholds(prev => ({
      ...prev,
      [field]: e.target.value as any
    }));

    // If any threshold is manually changed, set preset to custom
    setPreset('custom');
  };

  const handleSaveConfig = () => {
    // In a real implementation, this would save to backend
    alert('Custom alert thresholds saved successfully!');
    // Reset preset to custom since we manually configured
    setPreset('custom');
  };

  const handleResetToPreset = () => {
    handlePresetChange({ target: { value: preset } } as React.ChangeEvent<HTMLSelectElement>);
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <h2 className="text-3xl font-bold text-text-main mb-4">
          Custom Alert Thresholds
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Configure what constitutes breaking vs non-breaking changes for your team's context.
        </p>
      </div>

      {/* Preset Configurations */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-4">
          Preset Configurations
        </h3>
        <div className="grid gap-4 sm:grid-cols-4">
          <button
            className={`flex flex-col items-center space-y-3 p-4 rounded-lg border hover:bg-bg-hover transition-all ${preset === 'strict' ? 'border-accent-info bg-accent-info/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'strict' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-breaking text-2xl font-bold">■</span>
            <h4 className="font-semibold text-text-main">Strict</h4>
            <p className="text-text-sm text-text-muted text-center">
              All changes treated as breaking
            </p>
          </button>
          <button
            className={`flex flex-col items-center space-y-3 p-4 rounded-lg border hover:bg-bg-hover transition-all ${preset === 'recommended' ? 'border-accent-info bg-accent-info/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'recommended' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-warning text-2xl font-bold">▲</span>
            <h4 className="font-semibold text-text-main">Recommended</h4>
            <p className="text-text-sm text-text-muted text-center">
              Balanced approach for most teams
            </p>
          </button>
          <button
            className={`flex flex-col items-center space-y-3 p-4 rounded-lg border hover:bg-bg-hover transition-all ${preset === 'lenient' ? 'border-accent-info bg-accent-info/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'lenient' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-healthy text-2xl font-bold">●</span>
            <h4 className="font-semibold text-text-main">Lenient</h4>
            <p className="text-text-sm text-text-muted text-center">
              Only critical changes as breaking
            </p>
          </button>
          <button
            className={`flex flex-col items-center space-y-3 p-4 rounded-lg border hover:bg-bg-hover transition-all ${preset === 'custom' ? 'border-accent-info bg-accent-info/10' : 'border-transparent'}`}
            onClick={handleResetToPreset}
          >
            <span className="text-accent-info text-2xl font-bold">○</span>
            <h4 className="font-semibold text-text-main">Custom</h4>
            <p className="text-text-sm text-text-muted text-center">
              Manual configuration
            </p>
          </button>
        </div>
        <p className="mt-4 text-text-sm text-text-muted">
          Select a preset to quickly configure threshold settings for different team needs.
        </p>
      </div>

      {/* Per-Change Type Configuration */}
      <div className="bg-bg-card rounded-xl border border-border-color p-6">
        <h3 className="text-xl font-semibold text-text-main mb-6">
          Per-Change Type Configuration
        </h3>
        <div className="grid gap-6">
          {/* Type Changes */}
          <div className="space-y-4">
            <label className="block text-sm font-medium text-text-muted mb-2">
              Type Changes (int → string, etc.)
            </label>
            <select
              value={thresholds.typeChange}
              onChange={(e) => handleThresholdChange(e, 'typeChange')}
              className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </div>

          {/* Required → Optional Fields */}
          <div className="space-y-4">
            <label className="block text-sm font-medium text-text-muted mb-2">
              Required → Optional Fields
            </label>
            <select
              value={thresholds.requiredOptional}
              onChange={(e) => handleThresholdChange(e, 'requiredOptional')}
              className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </div>

          {/* Added/Removed Fields */}
          <div className="space-y-4">
            <label className="block text-sm font-medium text-text-muted mb-2">
              Added/Removed Fields
            </label>
            <select
              value={thresholds.addedRemoved}
              onChange={(e) => handleThresholdChange(e, 'addedRemoved')}
              className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </div>

          {/* Status Code Changes */}
          <div className="space-y-4">
            <label className="block text-sm font-medium text-text-muted mb-2">
              Status Code Changes
            </label>
            <select
              value={thresholds.statusCode}
              onChange={(e) => handleThresholdChange(e, 'statusCode')}
              className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </div>

          {/* Header Changes */}
          <div className="space-y-4">
            <label className="block text-sm font-medium text-text-muted mb-2">
              Header Changes
            </label>
            <select
              value={thresholds.headerChanges}
              onChange={(e) => handleThresholdChange(e, 'headerChanges')}
              className="w-full px-4 py-3 rounded-lg border border-border-color bg-bg-hover text-text-main focus:outline-none focus:ring-2 focus:ring-accent-info focus:border-transparent"
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4 pt-6">
        <button
          onClick={handleResetToPreset}
          className="px-6 py-3 rounded-lg border border-border-color text-text-main hover:bg-bg-hover transition-colors flex-1 sm:auto"
        >
          Reset to Preset
        </button>
        <button
          onClick={handleSaveConfig}
          className="px-6 py-3 rounded-lg bg-accent-info text-white hover:bg-accent-info/90 transition-colors flex-1 sm:auto"
        >
          Save Configuration
        </button>
      </div>
    </div>
  );
};

export default CustomAlertThresholds;