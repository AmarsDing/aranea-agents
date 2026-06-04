# Memory Admin Interfaces

## Memory Admin Interfaces

### Requirement: SessionAdminStore deprecated interface migration
The `biz.SessionAdminStore` deprecated composite interface SHALL be replaced by its constituent sub-interfaces in all Wire bindings. Code that depends on `SessionAdminStore` SHALL be migrated to depend on the specific sub-interfaces it needs (e.g., `L0AdminStore`, `L1TaskWriter`, `L3FactReader`).

#### Scenario: Wire binding uses specific sub-interface
- **WHEN** a usecase needs L0 snapshot operations
- **THEN** it SHALL depend on `biz.L0AdminStore`, NOT `biz.SessionAdminStore`

#### Scenario: SessionAdminStore removed from Wire
- **WHEN** all consumers have been migrated to specific sub-interfaces
- **THEN** `biz.SessionAdminStore` SHALL be deleted

### Requirement: CascadeGraphStore split into sub-interfaces
The `biz.CascadeGraphStore` composite interface SHALL be split into `CascadeProposalRepo` and `CascadeSagaRepo`. Consumers SHALL depend on the specific sub-interface they need.

#### Scenario: Cascade proposal operations
- **WHEN** a usecase needs cascade proposal CRUD
- **THEN** it SHALL depend on `biz.CascadeProposalRepo` with methods: `InsertCascadeProposal`, `GetCascadeProposalRow`, `ListCascadeProposalRows`, `UpdateCascadeProposalStatus`

#### Scenario: Cascade saga operations
- **WHEN** a usecase needs cascade saga step management
- **THEN** it SHALL depend on `biz.CascadeSagaRepo` with methods: `InitCascadeSagaSteps`, `GetCascadeSagaSteps`, `UpdateSagaStepState`, `UpdateSagaStepResult`, `HasCascadeSaga`

#### Scenario: CascadeGraphStore removed
- **WHEN** all consumers have been migrated
- **THEN** `biz.CascadeGraphStore` SHALL be deleted
