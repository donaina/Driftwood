import React, { useState } from 'react';
import { Button, Field, Panel, PanelTitle, inputClass, toast } from './ui';

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
    toast('Thresholds Saved', 'Custom alert thresholds have been saved.');
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
        <h2 className="text-2xl font-semibold tracking-tight text-text-main mb-4">
          Custom Alert Thresholds
        </h2>
        <p className="text-text-muted max-w-xl mx-auto">
          Configure what constitutes breaking vs non-breaking changes for your team's context.
        </p>
      </div>

      {/* Preset Configurations */}
      <Panel>
        <PanelTitle className="mb-4">
          Preset Configurations
        </PanelTitle>
        <div className="grid gap-4 sm:grid-cols-4">
          <button
            type="button"
            aria-pressed={preset === 'strict'}
            className={`flex flex-col items-center space-y-3 p-4 rounded-sm border transition-all duration-100 active:translate-y-px hover:bg-bg-hover ${preset === 'strict' ? 'border-accent-healthy bg-accent-healthy/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'strict' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-breaking text-2xl font-semibold">■</span>
            <PanelTitle level={4} size="base">Strict</PanelTitle>
            <p className="text-text-sm text-text-muted text-center">
              All changes treated as breaking
            </p>
          </button>
          <button
            type="button"
            aria-pressed={preset === 'recommended'}
            className={`flex flex-col items-center space-y-3 p-4 rounded-sm border transition-all duration-100 active:translate-y-px hover:bg-bg-hover ${preset === 'recommended' ? 'border-accent-healthy bg-accent-healthy/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'recommended' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-warning text-2xl font-semibold">▲</span>
            <PanelTitle level={4} size="base">Recommended</PanelTitle>
            <p className="text-text-sm text-text-muted text-center">
              Balanced approach for most teams
            </p>
          </button>
          <button
            type="button"
            aria-pressed={preset === 'lenient'}
            className={`flex flex-col items-center space-y-3 p-4 rounded-sm border transition-all duration-100 active:translate-y-px hover:bg-bg-hover ${preset === 'lenient' ? 'border-accent-healthy bg-accent-healthy/10' : 'border-transparent'}`}
            onClick={() => handlePresetChange({ target: { value: 'lenient' } } as React.ChangeEvent<HTMLSelectElement>)}
          >
            <span className="text-accent-healthy text-2xl font-semibold">●</span>
            <PanelTitle level={4} size="base">Lenient</PanelTitle>
            <p className="text-text-sm text-text-muted text-center">
              Only critical changes as breaking
            </p>
          </button>
          <button
            type="button"
            aria-pressed={preset === 'custom'}
            className={`flex flex-col items-center space-y-3 p-4 rounded-sm border transition-all duration-100 active:translate-y-px hover:bg-bg-hover ${preset === 'custom' ? 'border-accent-healthy bg-accent-healthy/10' : 'border-transparent'}`}
            onClick={handleResetToPreset}
          >
            <span className="text-accent-info text-2xl font-semibold">○</span>
            <PanelTitle level={4} size="base">Custom</PanelTitle>
            <p className="text-text-sm text-text-muted text-center">
              Manual configuration
            </p>
          </button>
        </div>
        <p className="mt-4 text-text-sm text-text-muted">
          Select a preset to quickly configure threshold settings for different team needs.
        </p>
      </Panel>

      {/* Per-Change Type Configuration */}
      <Panel>
        <PanelTitle className="mb-6">
          Per-Change Type Configuration
        </PanelTitle>
        <div className="grid gap-6">
          {/* Type Changes */}
          <Field label="Type Changes (int → string, etc.)">
            <select
              value={thresholds.typeChange}
              onChange={(e) => handleThresholdChange(e, 'typeChange')}
              className={inputClass}
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>

          {/* Required → Optional Fields */}
          <Field label="Required → Optional Fields">
            <select
              value={thresholds.requiredOptional}
              onChange={(e) => handleThresholdChange(e, 'requiredOptional')}
              className={inputClass}
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>

          {/* Added/Removed Fields */}
          <Field label="Added/Removed Fields">
            <select
              value={thresholds.addedRemoved}
              onChange={(e) => handleThresholdChange(e, 'addedRemoved')}
              className={inputClass}
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>

          {/* Status Code Changes */}
          <Field label="Status Code Changes">
            <select
              value={thresholds.statusCode}
              onChange={(e) => handleThresholdChange(e, 'statusCode')}
              className={inputClass}
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>

          {/* Header Changes */}
          <Field label="Header Changes">
            <select
              value={thresholds.headerChanges}
              onChange={(e) => handleThresholdChange(e, 'headerChanges')}
              className={inputClass}
            >
              <option value="breaking">Breaking</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>
        </div>
      </Panel>

      {/* Actions */}
      <div className="flex flex-col sm:flex-row sm:justify-end sm:gap-4 pt-6">
        <Button className="flex-1 sm:w-auto" onClick={handleResetToPreset}>
          Reset to Preset
        </Button>
        <Button
          variant="primary"
          className="flex-1 sm:w-auto"
          onClick={handleSaveConfig}
        >
          Save Configuration
        </Button>
      </div>
    </div>
  );
};

export default CustomAlertThresholds;