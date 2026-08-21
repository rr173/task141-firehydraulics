// Package model defines the domain entities for the fire-sprinkler hydraulic
// design & compliance engine. Structs carry JSON tags so the HTTP layer and the
// selfcheck decode them with consistent field names; persistence rows are
// mapped 1:1 to these structs in the store layer.
//
// All numeric quantities are fixed-point integers (no float crosses the API
// boundary):
//   - pressure: millibar (1 bar = 1000 mbar)
//   - flow: litres per minute (L/min)
//   - length / elevation / diameter: millimetres (mm)
//   - area: square decimetres (dm² = 0.01 m²)
//   - density: mm/min (design density over the design area)
//   - k-factor, C-factor, hazard class: integers / enums
//   - time: epoch seconds
package model

// HazardClass is the NFPA 13 occupancy hazard classification.
type HazardClass string

const (
	HazardLight       HazardClass = "light"        // 轻危
	HazardOrdinary1   HazardClass = "ordinary_1"   // 普危Ⅰ
	HazardOrdinary2   HazardClass = "ordinary_2"   // 普危Ⅱ
	HazardExtra1      HazardClass = "extra_1"      // 严危Ⅰ
	HazardExtra2      HazardClass = "extra_2"      // 严危Ⅱ
	HazardStorage     HazardClass = "storage"     // 仓库
)

// SystemKind distinguishes sprinkler vs standpipe systems.
type SystemKind string

const (
	KindSprinkler  SystemKind = "sprinkler"  // 自动喷淋系统
	KindStandpipe  SystemKind = "standpipe"  // 消火栓系统
)

// NodeType classifies a node in the hydraulic network.
type NodeType string

const (
	NodeSource     NodeType = "source"     // 水源点(管网根)
	NodeBase       NodeType = "base"       // 基准点(系统 riser 底)
	NodeJunction   NodeType = "junction"   // 三通/弯头/变径
	NodeSprinkler  NodeType = "sprinkler"  // 喷头
	NodeStandpipe  NodeType = "standpipe"  // 消火栓接口
	NodeDrain      NodeType = "drain"      // 排水/试验接口
)

// SystemState is the lifecycle state of a fire-protection system.
type SystemState string

const (
	StateDraft      SystemState = "draft"
	StateDesigned   SystemState = "designed"
	StateSubmitted  SystemState = "submitted"
	StateApproved   SystemState = "approved"
	StateInstalled  SystemState = "installed"
	StateHydrostatic SystemState = "hydrostatic"
	StateAccepted   SystemState = "accepted"
	StateInService  SystemState = "in_service"
	StateImpaired   SystemState = "impaired"
	StateRestored   SystemState = "restored"
)

// WaterSupplyKind is the source of system water.
type WaterSupplyKind string

const (
	SupplyCity     WaterSupplyKind = "city"      // 市政管网
	SupplyTank     WaterSupplyKind = "tank"      // 消防水箱
	SupplyReservoir WaterSupplyKind = "reservoir" // 天然水源/水池
)

// PumpDriverType is the fire-pump driver.
type PumpDriverType string

const (
	DriverElectric    PumpDriverType = "electric"   // 电动机
	DriverDiesel      PumpDriverType = "diesel"      // 柴油机
	DriverDual        PumpDriverType = "dual"        // 双驱动
)

// ImpairmentStatus is the state of a system impairment.
type ImpairmentStatus string

const (
	ImpairmentActive   ImpairmentStatus = "active"
	ImpairmentRestored ImpairmentStatus = "restored"
)

// CompensatingKind is the compensating measure during an impairment.
type CompensatingKind string

const (
	CompPatrol         CompensatingKind = "patrol"            // 巡逻
	CompTemporaryPipe  CompensatingKind = "temporary_pipe"    // 临时管
	CompManualFireWatch CompensatingKind = "manual_fire_watch" // 人工消防值守
)

// InspectionKind is the periodic inspection/test category.
type InspectionKind string

const (
	InspectMonthly   InspectionKind = "monthly"
	InspectQuarterly InspectionKind = "quarterly"
	InspectAnnual    InspectionKind = "annual"
	InspectFlowTest  InspectionKind = "flow_test"
	InspectPumpTest  InspectionKind = "pump_test"
)

// Project is the top-level fire-protection project (a building / site).
type Project struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	HazardClass   HazardClass `json:"hazard_class"`
	DesignDate    int64      `json:"design_date"`     // epoch day
	CreatedAt     int64      `json:"created_at"`      // epoch seconds
}

// System is a fire-protection system (sprinkler or standpipe).
type System struct {
	ID                   string      `json:"id"`
	ProjectID            string      `json:"project_id"`
	Kind                 SystemKind  `json:"kind"`
	Name                 string      `json:"name"`
	BaseElevationMM      int64       `json:"base_elevation_mm"` // mm above datum
	DesignAreaDM2        int64       `json:"design_area_dm2"`   // dm² = 0.01 m²
	DesignDensity         int64       `json:"design_density"`     // mm/min
	PerHeadCoverageDM2    int64       `json:"per_head_coverage_dm2"` // dm² per sprinkler (NFPA spacing)
	RemoteNodeID         string      `json:"remote_node_id,omitempty"` // most unfavourable node
	State                SystemState `json:"state"`
	WaterSupplyID        string      `json:"water_supply_id,omitempty"`
	PumpID               string      `json:"pump_id,omitempty"`
	UpdatedAt           int64       `json:"updated_at"`
}

// Node is a point in the hydraulic network.
type Node struct {
	ID                 string   `json:"id"`
	SystemID           string   `json:"system_id"`
	Type               NodeType `json:"type"`
	Label              string   `json:"label"`
	ElevationMM        int64    `json:"elevation_mm"`        // mm
	KFactor            int64    `json:"k_factor,omitempty"` // 公制 K (e.g. 80)
	DesignMinPressure  int64    `json:"design_min_pressure_mbar,omitempty"` // mbar
	Seq                int64    `json:"seq"` // topology order
}

// PipeSegment connects two nodes; flow goes upstream→downstream-toward-source.
// DownstreamNode is toward the sprinklers; UpstreamNode toward the source.
type PipeSegment struct {
	ID             string `json:"id"`
	SystemID       string `json:"system_id"`
	UpstreamNodeID string `json:"upstream_node_id"` // toward source
	DownstreamNodeID string `json:"downstream_node_id"` // toward sprinklers
	NominalDiaMM   int64  `json:"nominal_dia_mm"`   // 公称直径
	InnerDiaMM     int64  `json:"inner_dia_mm"`     // 实际内径
	LengthMM       int64  `json:"length_mm"`        // 实际管长
	CFactor        int64  `json:"c_factor"`          // Hazen-Williams C
	FittingEquivMM int64  `json:"fitting_equiv_mm"`  // 管件当量长度
	Seq            int64  `json:"seq"`
}

// WaterSupply is the source water supply for a system.
type WaterSupply struct {
	ID               string           `json:"id"`
	SystemID         string           `json:"system_id"`
	Kind             WaterSupplyKind  `json:"kind"`
	StaticPressure   int64            `json:"static_pressure_mbar"` // 静压 mbar
	Points           []SupplyPoint    `json:"points,omitempty"`    // 残压曲线
}

// SupplyPoint is one (flow → residual pressure) point on a supply curve.
type SupplyPoint struct {
	ID        string `json:"id,omitempty"`
	SupplyID  string `json:"supply_id,omitempty"`
	FlowLPM   int64  `json:"flow_lpm"`              // L/min
	Pressure  int64  `json:"residual_pressure_mbar"` // 残压 mbar
	Seq       int64  `json:"seq"`
}

// FirePump is the fire pump serving a system (optional).
type FirePump struct {
	ID                    string          `json:"id"`
	SystemID              string          `json:"system_id"`
	RatedFlowLPM          int64           `json:"rated_flow_lpm"`           // 额定流量
	RatedHeadMbar         int64           `json:"rated_head_mbar"`          // 额定扬程
	ChurnPressureMbar     int64           `json:"churn_pressure_mbar"`      // 零流量堵转压力
	FiftyExtraFlowLPM     int64           `json:"hundred_fifty_flow_lpm"`   // 150% 点流量
	FiftyExtraHeadMbar    int64           `json:"hundred_fifty_head_mbar"` // 150% 点压力
	DriverType            PumpDriverType  `json:"driver_type"`
	RatedRPM              int64           `json:"rated_rpm"`
}

// HydraulicResult is the computed node table for a system.
type HydraulicResult struct {
	ID                  string      `json:"id"`
	SystemID            string      `json:"system_id"`
	BaseFlowLPM         int64       `json:"base_flow_lpm"`          // 基准点总流量
	BaseRequiredPressure int64      `json:"base_required_pressure_mbar"` // 基准点所需压力
	RemotePressureMbar  int64       `json:"remote_pressure_mbar"`  // 最不利点压力
	RemoteFlowLPM       int64       `json:"remote_flow_lpm"`       // 最不利点流量
	Nodes               []NodeResult `json:"nodes"`
	CalcEpoch           int64       `json:"calc_epoch"`
}

// NodeResult is the computed pressure/flow at one node.
type NodeResult struct {
	NodeID    string `json:"node_id"`
	Label     string `json:"label"`
	PressureMbar int64 `json:"pressure_mbar"`
	FlowLPM   int64  `json:"flow_lpm,omitempty"` // sprinkler flow at this node
	ElevationMM int64 `json:"elevation_mm"`
}

// SupplyComparison is the water-supply adequacy comparison at the base point.
type SupplyComparison struct {
	ID                 string `json:"id"`
	SystemID           string `json:"system_id"`
	BaseFlowLPM        int64  `json:"base_flow_lpm"`
	AvailablePressure  int64  `json:"available_pressure_mbar"` // supply at Q (or +pump)
	RequiredPressure   int64  `json:"required_pressure_mbar"`
	SurplusMbar        int64  `json:"surplus_mbar"`            // available - required (may be <0)
	NeedsPump          bool   `json:"needs_pump"`
	PumpAdded          bool   `json:"pump_added"`
	CheckedEpoch       int64 `json:"checked_epoch"`
}

// ComplianceCheck is one NFPA rule result.
type ComplianceCheck struct {
	ID          string `json:"id"`
	SystemID    string `json:"system_id"`
	RuleCode    string `json:"rule_code"`
	Passed      bool   `json:"passed"`
	Detail      string `json:"detail,omitempty"`
	CheckedEpoch int64 `json:"checked_epoch"`
}

// HydrostaticTest is a pressure test record.
type HydrostaticTest struct {
	ID              string `json:"id"`
	SystemID        string `json:"system_id"`
	TestPressureMbar int64 `json:"test_pressure_mbar"`
	HoldSeconds     int64  `json:"hold_seconds"`
	Leaked          bool   `json:"leaked"`
	Passed          bool   `json:"passed"`
	TestEpoch       int64  `json:"test_epoch"`
}

// AcceptanceRecord is the acceptance (commissioning) record.
type AcceptanceRecord struct {
	ID          string   `json:"id"`
	SystemID    string   `json:"system_id"`
	Conclusion  string   `json:"conclusion"` // pass / fail
	Defects     []string `json:"defects"`
	AcceptedBy  string   `json:"accepted_by"`
	AcceptedEpoch int64  `json:"accepted_epoch"`
}

// InspectionRecord is a periodic inspection/test record.
type InspectionRecord struct {
	ID        string `json:"id"`
	SystemID  string `json:"system_id"`
	Kind      InspectionKind `json:"kind"`
	Result    string `json:"result"` // pass / fail / na
	Detail    string `json:"detail,omitempty"`
	InspectedEpoch int64 `json:"inspected_epoch"`
}

// Impairment is a system-out-of-service event.
type Impairment struct {
	ID                 string           `json:"id"`
	SystemID           string           `json:"system_id"`
	Scope              string           `json:"scope"`
	Reason             string           `json:"reason"`
	StartedEpoch       int64            `json:"started_epoch"`
	ExpectedRestoreEpoch int64          `json:"expected_restore_epoch"`
	ActualRestoreEpoch int64           `json:"actual_restore_epoch,omitempty"`
	Status             ImpairmentStatus `json:"status"`
	Measures           []CompensatingMeasure `json:"measures,omitempty"`
}

// CompensatingMeasure is the mitigation during an impairment.
type CompensatingMeasure struct {
	ID           string           `json:"id"`
	ImpairmentID string           `json:"impairment_id"`
	Kind         CompensatingKind `json:"kind"`
	Owner        string           `json:"owner"`
	StartedEpoch int64            `json:"started_epoch"`
	EndedEpoch   int64            `json:"ended_epoch,omitempty"`
}

// LifecycleEvent is one recorded state transition (event-stream source of truth).
type LifecycleEvent struct {
	ID         string `json:"id"`
	SystemID   string `json:"system_id"`
	FromState  SystemState `json:"from_state"`
	ToState    SystemState `json:"to_state"`
	Reason     string `json:"reason"`
	EventEpoch int64  `json:"event_epoch"`
}

// FullReport bundles every computed view for a system, used by the frontend and
// the /full-report API.
type FullReport struct {
	System      *System           `json:"system"`
	Nodes       []Node            `json:"nodes"`
	Pipes       []PipeSegment     `json:"pipes"`
	WaterSupply *WaterSupply      `json:"water_supply,omitempty"`
	Pump        *FirePump         `json:"pump,omitempty"`
	Hydraulic   *HydraulicResult  `json:"hydraulic,omitempty"`
	Supply      *SupplyComparison `json:"supply_comparison,omitempty"`
	Compliance  []ComplianceCheck `json:"compliance"`
	Lifecycle   []LifecycleEvent  `json:"lifecycle"`
}
