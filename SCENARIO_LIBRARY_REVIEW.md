# Scenario Library Review - Driftwood Medium-Term Improvement

## Overview
Review of the Scenario Library feature in Driftwood, which is the second medium-term improvement listed in MEDIUM_TERM_IMPROVEMENTS.md. The scenario library provides pre-configured API breaking change scenarios for learning and experimentation.

## Current Implementation Analysis

### What's Working Well
1. **Complete Scenario Structure**: Each scenario includes:
   - Unique ID
   - Title and description
   - Severity level (breaking, warning, info)
   - API type (REST, GraphQL)
   - Impact assessment
   - Fix guidance
   - Specific changes (field-level modifications)

2. **Filtering Functionality**: 
   - Filter buttons for All/Breaking/Warning/Info scenarios
   - Active filter state management
   - Dynamic re-rendering when filters change

3. **Visual Design**:
   - Scenario cards with severity badges (using Vital Teal/Fault Red/Caution Amber)
   - Clear hierarchy: title → description → details → action button
   - Consistent spacing and typography

4. **Activation System**:
   - "Load Scenario" buttons for each scenario
   - loadScenario() function that handles scenario activation
   - Integration with mock simulator for compatible scenarios
   - Toast notifications upon scenario loading

5. **Existing Scenarios (6 total)**:
   - Breaking: Type change (number→string), Removed required field, GraphQL type rename
   - Warning: Status code change (200→201), Header removal (X-RateLimit-Remaining)
   - Info: Added optional field (phone number)

### Gaps vs. Medium-Term Improvements Document

#### 1. Missing gRPC Support
The document specifies filtering by type: "REST, GraphQL, gRPC" but currently only REST and GraphQL scenarios exist.

#### 2. Incomplete Mock Simulator Integration
Currently only 3 of 6 scenarios automatically configure the mock simulator:
- ✅ type-change-number-to-string → TYPE_BREAK mode
- ✅ removed-required-field → MISSING_FIELD mode  
- ❌ status-code-change → Manual setup instructions only
- ✅ added-optional-field → NORMAL mode
- ❌ graphql-type-rename → Manual setup (GraphQL not supported by mock simulator)
- ❌ header-removal → Manual setup instructions only

#### 3. Missing User-Created Scenario Sharing
The document mentions "User-created scenario sharing" as a feature, but this functionality is not implemented.

#### 4. Limited Scenario Details
While scenarios include impact and fix information, they could benefit from:
- More detailed change descriptions
- Code examples showing before/after
- Difficulty level or estimated fix time
- Related scenarios or patterns

### Specific Improvement Opportunities

#### Enhance Load Scenario Function
Update loadScenario() to:
1. Add support for status-code-change scenario (if mock mode supports it)
2. Improve instructions for scenarios requiring manual setup
3. Consider adding new mock modes if needed for header/status code changes
4. Provide better feedback when scenarios require manual configuration

#### Add gRPC Scenarios
Create scenarios demonstrating gRPC-specific breaking changes:
- Changing method signatures (parameter types, return types)
- Removing required fields in protobuf messages
- Changing enum values
- Changing service method routing

#### Improve Scenario Cards
Enhance card design to:
- Show more prominent severity indicators
- Include visual diff examples
- Add "difficulty" or "impact" ratings
- Show related scenario connections

#### Add User Scenario Features
Implement:
- "Create Scenario" button/form
- Scenario export/import functionality
- Sharing via URL parameters or direct links
- Community scenario templates

### Alignment with Enterprise Software Principles

The scenario library already aligns well with enterprise principles:
- **Precision Instrument Approach**: Focused, technical scenarios with clear cause-effect
- **Progressive Disclosure**: Basic info visible, details available on interaction
- **Error Prevention**: Clear instructions prevent incorrect scenario setup
- **Power User Orientation**: Efficient one-click activation for experienced users
- **Cognitive Load Reduction**: Consistent structure reduces learning curve

### Recommended Next Steps

1. **Immediate Enhancements** (can be done frontend-only):
   - Improve loadScenario() messaging and instructions
   - Add better tooltip/help text for manual setup scenarios
   - Enhance scenario card visual design
   - Add scenario difficulty/impact ratings

2. **Medium Complexity** (may require backend changes):
   - Add gRPC scenarios
   - Implement new mock modes for unsupported scenario types
   - Add scenario export/import functionality

3. **Higher Complexity** (requires significant changes):
   - User-created scenario sharing
   - Scenario collaboration features
   - Advanced scenario customization

### Current Status
The scenario library provides a solid foundation that meets most core requirements. The primary gaps are in mock simulator integration completeness and missing gRPC support, which prevent fully realizing the "one-click activation" promise for all scenarios.