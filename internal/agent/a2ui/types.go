package a2ui

import "time"

type Plan struct {
	ID           string
	Goal         string
	Steps        []PlanStep
	Dependencies map[string][]string
	CreatedAt    time.Time
}

type PlanStep struct {
	ID          string
	Name        string
	Description string
	AgentName   string
	Tools       []string
	DependsOn   []string
}

type UserAction struct {
	Name              string         `json:"name"`
	SurfaceID         string         `json:"surfaceId"`
	SourceComponentID string         `json:"sourceComponentId"`
	Timestamp         time.Time      `json:"timestamp"`
	Context           map[string]any `json:"context"`
}

type BeginRendering struct {
	SurfaceID string       `json:"surfaceId"`
	Root      string       `json:"root"`
	Styles    *SurfaceStyles `json:"styles,omitempty"`
}

type SurfaceStyles struct {
	Font        string `json:"font,omitempty"`
	PrimaryColor string `json:"primaryColor,omitempty"`
}

type SurfaceUpdate struct {
	SurfaceID  string      `json:"surfaceId"`
	Components []Component `json:"components"`
}

type DataModelUpdate struct {
	SurfaceID string      `json:"surfaceId"`
	Path      string      `json:"path,omitempty"`
	Contents  []DataEntry `json:"contents"`
}

type DeleteSurface struct {
	SurfaceID string `json:"surfaceId"`
}

type Component struct {
	ID        string         `json:"id"`
	Weight    *float64       `json:"weight,omitempty"`
	Component ComponentBody  `json:"component"`
}

type ComponentBody struct {
	Text             *TextComponent             `json:"Text,omitempty"`
	Image            *ImageComponent            `json:"Image,omitempty"`
	Icon             *IconComponent             `json:"Icon,omitempty"`
	Video            *VideoComponent            `json:"Video,omitempty"`
	AudioPlayer      *AudioPlayerComponent      `json:"AudioPlayer,omitempty"`
	Row              *RowComponent              `json:"Row,omitempty"`
	Column           *ColumnComponent           `json:"Column,omitempty"`
	List             *ListComponent             `json:"List,omitempty"`
	Card             *CardComponent             `json:"Card,omitempty"`
	Tabs             *TabsComponent             `json:"Tabs,omitempty"`
	Divider          *DividerComponent          `json:"Divider,omitempty"`
	Modal            *ModalComponent            `json:"Modal,omitempty"`
	Button           *ButtonComponent           `json:"Button,omitempty"`
	CheckBox         *CheckBoxComponent         `json:"CheckBox,omitempty"`
	TextField        *TextFieldComponent        `json:"TextField,omitempty"`
	DateTimeInput    *DateTimeInputComponent    `json:"DateTimeInput,omitempty"`
	MultipleChoice   *MultipleChoiceComponent   `json:"MultipleChoice,omitempty"`
	Slider           *SliderComponent           `json:"Slider,omitempty"`
}

type DataBinding struct {
	LiteralString  *string  `json:"literalString,omitempty"`
	Path           *string  `json:"path,omitempty"`
	LiteralNumber  *float64 `json:"literalNumber,omitempty"`
	LiteralBoolean *bool    `json:"literalBoolean,omitempty"`
}

func StrPtr(s string) *string    { return &s }
func NumPtr(n float64) *float64  { return &n }
func BoolPtr(b bool) *bool       { return &b }

type TextComponent struct {
	Text       DataBinding `json:"text"`
	UsageHint  string      `json:"usageHint,omitempty"`
}

type ImageComponent struct {
	URL       DataBinding `json:"url"`
	Fit       string      `json:"fit,omitempty"`
	UsageHint string      `json:"usageHint,omitempty"`
}

type IconComponent struct {
	Name DataBinding `json:"name"`
}

type VideoComponent struct {
	URL DataBinding `json:"url"`
}

type AudioPlayerComponent struct {
	URL         DataBinding `json:"url"`
	Description DataBinding `json:"description,omitempty"`
}

type ChildrenDef struct {
	ExplicitList []string       `json:"explicitList,omitempty"`
	Template     *TemplateDef   `json:"template,omitempty"`
}

type TemplateDef struct {
	ComponentID string `json:"componentId"`
	DataBinding string `json:"dataBinding"`
}

type RowComponent struct {
	Children     ChildrenDef `json:"children"`
	Distribution string      `json:"distribution,omitempty"`
	Alignment    string      `json:"alignment,omitempty"`
}

type ColumnComponent struct {
	Children     ChildrenDef `json:"children"`
	Distribution string      `json:"distribution,omitempty"`
	Alignment    string      `json:"alignment,omitempty"`
}

type ListComponent struct {
	Children  ChildrenDef `json:"children"`
	Direction string      `json:"direction,omitempty"`
	Alignment string      `json:"alignment,omitempty"`
}

type CardComponent struct {
	Child string `json:"child"`
}

type TabItem struct {
	Title DataBinding `json:"title"`
	Child string      `json:"child"`
}

type TabsComponent struct {
	TabItems []TabItem `json:"tabItems"`
}

type DividerComponent struct {
	Axis string `json:"axis,omitempty"`
}

type ModalComponent struct {
	EntryPointChild string `json:"entryPointChild"`
	ContentChild    string `json:"contentChild"`
}

type ActionContextEntry struct {
	Key   string      `json:"key"`
	Value DataBinding `json:"value"`
}

type ActionDef struct {
	Name    string              `json:"name"`
	Context []ActionContextEntry `json:"context,omitempty"`
}

type ButtonComponent struct {
	Child  string     `json:"child"`
	Primary bool      `json:"primary,omitempty"`
	Action  ActionDef `json:"action"`
}

type CheckBoxComponent struct {
	Label DataBinding `json:"label"`
	Value DataBinding `json:"value"`
}

type TextFieldComponent struct {
	Label            DataBinding `json:"label"`
	Text             DataBinding `json:"text,omitempty"`
	TextFieldType    string      `json:"textFieldType,omitempty"`
	ValidationRegexp string      `json:"validationRegexp,omitempty"`
}

type DateTimeInputComponent struct {
	Value      DataBinding `json:"value"`
	EnableDate bool        `json:"enableDate,omitempty"`
	EnableTime bool        `json:"enableTime,omitempty"`
}

type ChoiceOption struct {
	Label DataBinding `json:"label"`
	Value string      `json:"value"`
}

type MultipleChoiceComponent struct {
	Selections           DataBinding    `json:"selections"`
	Options              []ChoiceOption `json:"options"`
	MaxAllowedSelections int            `json:"maxAllowedSelections,omitempty"`
}

type SliderComponent struct {
	Value    DataBinding `json:"value"`
	MinValue float64     `json:"minValue,omitempty"`
	MaxValue float64     `json:"maxValue,omitempty"`
}

type DataEntry struct {
	Key          string        `json:"key"`
	ValueString  *string       `json:"valueString,omitempty"`
	ValueNumber  *float64      `json:"valueNumber,omitempty"`
	ValueBoolean *bool         `json:"valueBoolean,omitempty"`
	ValueMap     []DataEntry   `json:"valueMap,omitempty"`
}
