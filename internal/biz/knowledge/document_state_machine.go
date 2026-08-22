package knowledge

import (
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// Document status machine (AS-FSM-01).
//
//	```mermaid
//	stateDiagram-v2
//	  [*] --> pending
//	  pending --> indexing : start
//	  pending --> error : fail
//	  indexing --> indexed : complete
//	  indexing --> error : fail
//	  indexing --> pending : reset
//	  indexed --> indexing : start
//	  indexed --> error : fail
//	  indexed --> pending : reset
//	  error --> indexing : start
//	  error --> pending : reset
//	```
//
// indexed and error are retryable (re-embed / repair). pending→indexed is
// rejected so callers must pass through indexing.

// DocumentState enumerates legal knowledge_documents.status values.
// Stability:evolving
type DocumentState string

const (
	DocumentStateNone     DocumentState = ""
	DocumentStatePending  DocumentState = "pending"
	DocumentStateIndexing DocumentState = "indexing"
	DocumentStateIndexed  DocumentState = "indexed"
	DocumentStateError    DocumentState = "error"
)

// DocumentEvent enumerates status-transition triggers.
// Stability:evolving
type DocumentEvent string

const (
	DocumentEventStart    DocumentEvent = "start"
	DocumentEventComplete DocumentEvent = "complete"
	DocumentEventFail     DocumentEvent = "fail"
	DocumentEventReset    DocumentEvent = "reset"
)

var documentTransitionRules = []shared.TransitionRule[DocumentState, DocumentEvent]{
	{From: DocumentStatePending, Event: DocumentEventStart, To: DocumentStateIndexing},
	{From: DocumentStatePending, Event: DocumentEventFail, To: DocumentStateError},
	{From: DocumentStateIndexing, Event: DocumentEventComplete, To: DocumentStateIndexed},
	{From: DocumentStateIndexing, Event: DocumentEventFail, To: DocumentStateError},
	{From: DocumentStateIndexing, Event: DocumentEventReset, To: DocumentStatePending},
	{From: DocumentStateIndexed, Event: DocumentEventStart, To: DocumentStateIndexing},
	{From: DocumentStateIndexed, Event: DocumentEventFail, To: DocumentStateError},
	{From: DocumentStateIndexed, Event: DocumentEventReset, To: DocumentStatePending},
	{From: DocumentStateError, Event: DocumentEventStart, To: DocumentStateIndexing},
	{From: DocumentStateError, Event: DocumentEventReset, To: DocumentStatePending},
}

// DocumentStateMachine validates knowledge document status transitions.
// Stability:evolving
type DocumentStateMachine struct {
	inner *shared.GenericStateMachine[DocumentState, DocumentEvent]
}

// NewDocumentStateMachine returns the shared document status machine.
func NewDocumentStateMachine() *DocumentStateMachine {
	return &DocumentStateMachine{
		inner: shared.NewGenericStateMachine[DocumentState, DocumentEvent](documentTransitionRules),
	}
}

// Transition validates and returns the next document status.
func (sm *DocumentStateMachine) Transition(from DocumentState, event DocumentEvent) (DocumentState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether from→to is a legal document status change.
func (sm *DocumentStateMachine) CanTransition(from, to DocumentState) bool {
	return sm.inner.CanTransition(from, to)
}

var defaultDocumentStateMachine = NewDocumentStateMachine()

// ErrDocumentStatusConflict is returned when a CAS status update does not match.
var ErrDocumentStatusConflict = apierror.Conflict(apierror.DomainKnowledge, "document status changed concurrently")

// ErrInvalidDocumentStatus is returned when the target status is not a known state.
var ErrInvalidDocumentStatus = apierror.BadRequest(apierror.DomainKnowledge, "invalid document status")

// NormalizeDocumentState maps an empty stored status to pending (CreateDocument default).
func NormalizeDocumentState(status string) DocumentState {
	if status == "" {
		return DocumentStatePending
	}
	return DocumentState(status)
}

// documentEventFor infers the event for a desired status. Same-state and
// pending→indexed (skip indexing) return ok=false.
func documentEventFor(from, to DocumentState) (DocumentEvent, bool) {
	if from == to {
		return "", false
	}
	switch to {
	case DocumentStateIndexing:
		return DocumentEventStart, true
	case DocumentStateIndexed:
		if from != DocumentStateIndexing {
			return "", false
		}
		return DocumentEventComplete, true
	case DocumentStateError:
		return DocumentEventFail, true
	case DocumentStatePending:
		return DocumentEventReset, true
	default:
		return "", false
	}
}
