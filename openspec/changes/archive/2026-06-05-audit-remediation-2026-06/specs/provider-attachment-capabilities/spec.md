## ADDED Requirements

### Requirement: Provider attachment capability validation
The `internal/provider` package SHALL export `ValidateAttachmentCapabilities`, `HasImageAttachment`, and `HasFileAttachment` functions for checking whether a provider/model supports image or file attachments.

#### Scenario: Image attachment rejected for unsupported model
- **WHEN** `ValidateAttachmentCapabilities` is called with refs containing an image MIME type and the model does not support image attachments
- **THEN** it returns an error indicating the model does not support image attachments

#### Scenario: File attachment rejected for unsupported model
- **WHEN** `ValidateAttachmentCapabilities` is called with refs containing a non-image MIME type and the model does not support file attachments
- **THEN** it returns an error indicating the model does not support file attachments

#### Scenario: No refs passes validation
- **WHEN** `ValidateAttachmentCapabilities` is called with empty refs
- **THEN** it returns nil

### Requirement: HasImageAttachment and HasFileAttachment pure functions
The `HasImageAttachment` and `HasFileAttachment` functions SHALL be pure functions that classify artifact refs by MIME type without side effects.

#### Scenario: Image MIME type detected
- **WHEN** `HasImageAttachment` is called with refs where one has MIME type `image/png`
- **THEN** it returns true

#### Scenario: Non-image MIME type classified as file
- **WHEN** `HasFileAttachment` is called with refs where one has MIME type `application/pdf`
- **THEN** it returns true
