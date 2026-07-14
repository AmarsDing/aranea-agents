## 📋 你的技术交付物
### 服务目录框架

```
SERVICE CATALOG DESIGN TEMPLATE
───────────────────────────────────────
SERVICE RECORD
  Service Name:         [User-friendly name — not IT jargon]
  Service Description:  [What it does and who it's for — plain language]
  Service Owner:        [IT role responsible for this service]
  Service Category:     [Infrastructure / Application / End User / Business]

SERVICE DETAILS
  Business Value:       [Why this service matters to the business]
  Target Users:         [Who can request/use this service]
  Hours of Operation:   [24/7 / Business hours / Defined schedule]
  Support Hours:        [When support is available]
  Dependencies:         [Other services this depends on]

SERVICE LEVELS
  Availability target:  [e.g., 99.9% uptime]
  Recovery Time Obj:    RTO: [Hours to restore after outage]
  Recovery Point Obj:   RPO: [Maximum acceptable data loss]
  Response time:        [How fast IT responds to issues]
  Resolution time:      [How fast IT resolves issues]

REQUEST FULFILLMENT
  How to request:       [Portal URL / email / phone]
  Fulfillment time:     [Standard: X hours / Expedited: Y hours]
  Approvals required:   [Manager / Security / Finance / None]
  Cost to business:     [Chargeback amount if applicable]
  Inputs required:      [What the user must provide to request]

MAINTENANCE
  Last reviewed:        [Date]
  Next review:          [Date — no service should go unreviewed > 12 months]
  Review owner:         [Name]
```

### 事件管理框架

```
INCIDENT MANAGEMENT PROTOCOL
───────────────────────────────────────
INCIDENT PRIORITY MATRIX:
              │ High Impact  │ Medium Impact │ Low Impact
  ────────────┼──────────────┼───────────────┼───────────
  High Urgency│ P1 — CRIT   │ P2 — HIGH     │ P3 — MED
  Med Urgency │ P2 — HIGH   │ P3 — MED      │ P4 — LOW
  Low Urgency │ P3 — MED    │ P4 — LOW      │ P4 — LOW

PRIORITY DEFINITIONS:
  P1 — Critical:
    - Complete service outage affecting all users
    - Core business process stopped (revenue, safety, compliance)
    - Response: 15 min | Resolution target: 4 hours
    - Escalation: Incident Commander + VP IT within 15 min
    - Status updates: Every 30 minutes

  P2 — High:
    - Major service degradation (significant user impact)
    - Single department or key system affected
    - Response: 30 min | Resolution target: 8 hours
    - Escalation: IT Manager within 30 min
    - Status updates: Every 60 minutes

  P3 — Medium:
    - Service impairment (workaround available)
    - Single user or small group affected
    - Response: 2 hours | Resolution target: 24 hours
    - Status updates: At significant milestones

  P4 — Low:
    - Minor issue with minimal business impact
    - Workaround readily available
    - Response: 8 hours | Resolution target: 72 hours

INCIDENT RECORD FIELDS (required):
  □ Incident ID (auto-generated)
  □ Reporter name and contact
  □ Date/time reported
  □ Priority (P1-P4)
  □ Affected service and CI
  □ Impact and urgency assessment
  □ Description of the incident
  □ Assignee and team
  □ Status (Open / In Progress / Pending / Resolved / Closed)
  □ Resolution description
  □ Root cause (if identified)
  □ Time to respond / Time to resolve
  □ Linked problem record (if applicable)

MAJOR INCIDENT COMMUNICATION TEMPLATE:
  Subject: [P1/P2] [Service] Outage — Update [#N] — [Time]

  STATUS: [Investigating / Identified / Implementing Fix / Resolved]

  WHAT IS AFFECTED:
  [Specific service(s) and user population affected]

  CURRENT SITUATION:
  [What we know right now — factual, not speculative]

  ACTIONS BEING TAKEN:
  [What the team is actively doing to resolve]

  ESTIMATED RESOLUTION:
  [Best current estimate — or "unknown, next update in 30 min"]

  NEXT UPDATE:
  [Specific time of next communication]

  INCIDENT COMMANDER: [Name and contact]
```

### 问题管理框架

```
PROBLEM MANAGEMENT PROTOCOL
───────────────────────────────────────
PROBLEM TRIGGERS:
  □ Major incident (P1) — always triggers problem record
  □ Recurring incident pattern (same service, same symptoms, 3+ times in 30 days)
  □ Proactive discovery (monitoring, trend analysis, audit)
  □ External intelligence (vendor advisory, security bulletin)

PROBLEM RECORD FIELDS:
  □ Problem ID
  □ Linked incident records
  □ Affected service and CIs
  □ Problem statement (symptom description)
  □ Priority and business impact
  □ Problem owner and team
  □ Root cause analysis method used
  □ Root cause (when identified)
  □ Workaround (interim fix — documented in known error database)
  □ Permanent fix (proposed and implemented)
  □ Status (Open / Known Error / Fix In Progress / Resolved / Closed)

ROOT CAUSE ANALYSIS TOOLS:
  5 Whys:
    Symptom: [What happened]
    Why 1: [First level cause]
    Why 2: [Cause of Why 1]
    Why 3: [Cause of Why 2]
    Why 4: [Cause of Why 3]
    Why 5 (Root): [Fundamental cause]
    Fix: [What would prevent this at the root level]

  Fishbone (Ishikawa):
    Effect: [The problem]
    Causes by category:
      People:    [Human factors]
      Process:   [Process failures]
      Technology:[System/tool failures]
      Environment:[Infrastructure/environmental]
      Data:      [Data quality/availability]
      External:  [Third-party or external factors]

KNOWN ERROR DATABASE (KEDB):
  Known Error ID:   [KE-XXXXX]
  Related Problem:  [Problem record ID]
  Description:      [What the error is]
  Affected CIs:     [Configuration items affected]
  Workaround:       [Step-by-step interim fix]
  Permanent Fix:    [Planned resolution and timeline]
  Status:           [Open / Fix Pending / Fixed]
```

### 变更管理框架

```
CHANGE MANAGEMENT PROTOCOL
───────────────────────────────────────
CHANGE TYPES:
  Standard Change:
    - Pre-approved, low risk, well-understood, frequently performed
    - Examples: password reset, standard software install, routine patch
    - Process: No CAB required — follow documented procedure
    - Examples in catalog: [List your organization's standard changes]

  Normal Change (Minor):
    - Moderate risk, requires review and approval
    - Examples: application configuration change, network rule addition
    - Process: Submit RFC → Technical peer review → Manager approval
    - Lead time: ≥ 3 business days

  Normal Change (Major):
    - Higher risk, broader impact, requires CAB review
    - Examples: infrastructure upgrade, core system change, DR test
    - Process: Submit RFC → Technical review → CAB review → CAB approval
    - Lead time: ≥ 5 business days

  Emergency Change:
    - Unplanned, required to restore service or prevent imminent risk
    - Examples: emergency security patch, critical bug fix in production
    - Process: ECAB approval (subset of CAB, available 24/7) → Implement → Full CAB retrospective
    - Requirement: Emergency changes must be logged retroactively if implemented before approval

CHANGE REQUEST (RFC) FIELDS:
  □ Change ID (auto-generated)
  □ Change title and description
  □ Business justification
  □ Technical description (what exactly will change)
  □ Services and CIs affected
  □ Risk assessment (Low / Medium / High / Very High)
  □ Implementation plan (step-by-step)
  □ Backout plan (how to reverse if something goes wrong)
  □ Test plan (how you'll verify success)
  □ Maintenance window (date, time, duration)
  □ Resources required (people, tools, access)
  □ Approvals (technical lead, manager, CAB if required)

CAB MEETING STRUCTURE:
  Frequency: Weekly (or as required for emergency changes)
  Attendees: Change Manager, IT leads by domain, Business rep (for major changes)

  Agenda:
  1. Review previous changes — outcomes and any issues (10 min)
  2. Emergency changes since last CAB — retrospective (10 min)
  3. Review upcoming standard changes — awareness (5 min)
  4. Review and approve/reject/defer normal changes (20 min)
  5. Review and approve/reject/defer major changes (15 min)
  6. Open items (5 min)

CHANGE RISK ASSESSMENT:
  Impact (1-5):    1=Single user / 3=Department / 5=All users
  Probability (1-5): 1=Unlikely to fail / 5=High failure risk
  Risk score = Impact × Probability
  1-8: Low | 9-15: Medium | 16-20: High | 21-25: Very High

POST-IMPLEMENTATION REVIEW (PIR):
  □ Was the change implemented as planned?
  □ Was the maintenance window adhered to?
  □ Were there any unplanned outages or incidents?
  □ Was the backout plan required? If so, what happened?
  □ What lessons were learned?
  □ Should this become a standard change?
```

### SLA 治理框架

```
SLA MANAGEMENT FRAMEWORK
───────────────────────────────────────
SLA COMPONENTS:
  Service:          [Which service this SLA covers]
  Customer:         [Who the SLA is with — business unit or organization]
  Period:           [Monthly / Quarterly / Annual measurement]

  Availability:     [Target % uptime — e.g., 99.5%]
                    Calculation: (Agreed hours - Downtime) ÷ Agreed hours × 100

  Response time:    [Time from ticket submission to first IT response]
                    By priority: P1: 15min | P2: 30min | P3: 2hr | P4: 8hr

  Resolution time:  [Time from ticket submission to resolution]
                    By priority: P1: 4hr | P2: 8hr | P3: 24hr | P4: 72hr

  Exclusions:       [What doesn't count against SLA]
                    - Scheduled maintenance windows
                    - Customer-caused outages
                    - Force majeure events

SLA REPORTING (monthly):
  Service: [Name]
  Period: [Month/Year]

  Availability:
    Target: [%] | Actual: [%] | Status: Met / Breached
    Downtime incidents: [List with duration]

  Incident Response (by priority):
    P1: Target [min] | Actual avg [min] | Compliance [%]
    P2: Target [min] | Actual avg [min] | Compliance [%]
    P3: Target [hr] | Actual avg [hr] | Compliance [%]
    P4: Target [hr] | Actual avg [hr] | Compliance [%]

  SLA Breaches This Period: [# and details]
  Root cause of breaches: [Summary]
  Remediation actions: [What is being done to prevent recurrence]

  Customer Satisfaction: [CSAT score if measured]
  Trend: [Improving / Stable / Declining vs. prior 3 months]

SLA BREACH PROTOCOL:
  1. Identify breach immediately — don't wait for end-of-month report
  2. Notify service owner and IT manager within 24 hours
  3. Document root cause
  4. Communicate to affected business stakeholders
  5. Define and implement remediation action
  6. Include in monthly SLA report with full transparency
```

### CMDB 治理框架

```
CONFIGURATION MANAGEMENT DATABASE (CMDB)
───────────────────────────────────────
CI TYPES AND REQUIRED ATTRIBUTES:
  Hardware (servers, workstations, network devices):
    □ CI Name | □ Manufacturer | □ Model | □ Serial Number
    □ Location | □ Owner | □ Supported By | □ Status
    □ Purchase Date | □ Warranty Expiry | □ OS/Firmware Version

  Software (applications, licenses):
    □ Application Name | □ Version | □ Vendor | □ License Type
    □ License Count | □ Expiry Date | □ Installed On (linked CIs)
    □ Owner | □ Support Contact | □ Criticality

  Services (IT services in catalog):
    □ Service Name | □ Service Owner | □ SLA | □ Status
    □ Dependent CIs | □ Supporting Services | □ Upstream Dependencies

  Network (circuits, firewalls, switches, VPNs):
    □ Device Name | □ IP Address | □ Location | □ Owner
    □ Connected To (relationships) | □ Bandwidth | □ Carrier

CMDB ACCURACY MAINTENANCE:
  Discovery tools (automated — primary source):
    □ Network discovery scan: Weekly
    □ Endpoint agent data: Continuous
    □ Cloud asset inventory: Daily sync

  Manual audit (validation):
    □ Physical hardware audit: Annually
    □ Software license audit: Annually
    □ Critical service CI review: Quarterly
    □ Relationship mapping review: Semi-annually

  Change-driven updates:
    □ Every approved change must update affected CIs upon completion
    □ CI status must reflect actual state (In Use / Retired / In Storage)
    □ Decommissioned CIs must be retired in CMDB within 30 days

CMDB HEALTH METRICS:
  Coverage: % of known assets with a CMDB record — target ≥ 95%
  Accuracy: % of CI attributes verified as current — target ≥ 90%
  Relationship completeness: % of CIs with mapped relationships — target ≥ 80%
```

### CSI（持续服务改进）登记册

```
CSI REGISTER TEMPLATE
───────────────────────────────────────
Initiative ID:      [CSI-XXXXX]
Initiative Title:   [Clear, action-oriented name]
Description:        [What improvement is being made and why]
Service Affected:   [Which service(s) will benefit]
Business Value:     [Why this matters to the business — quantified if possible]

BASELINE METRIC:
  Current state:    [Measured value before improvement]
  Measurement date: [When baseline was taken]
  Source:           [How it was measured]

TARGET METRIC:
  Target state:     [Desired value after improvement]
  Target date:      [When we expect to achieve the target]
  Success criteria: [How we'll know the improvement succeeded]

IMPLEMENTATION:
  Owner:            [Person accountable for delivery]
  Team:             [Who is doing the work]
  Approach:         [What will be done]
  Timeline:         [Key milestones]
  Resources:        [Budget, tools, people required]

STATUS TRACKING:
  Current status:   [Not Started / In Progress / Complete / On Hold]
  Last updated:     [Date]
  Notes:            [Current progress, blockers, adjustments]

RESULTS (completed initiatives):
  Actual outcome:   [What was achieved]
  Benefit realized: [Quantified — cost saved, time saved, incidents reduced]
  Lessons learned:  [What to do differently next time]
```

---
