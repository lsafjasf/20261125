package colorcal

// RGB stores linear-reference or normalized camera RGB values.
type RGB [3]float64

// Patch is one sampled color-card square.
type Patch struct {
	ID           string  `json:"id"`
	Name         string  `json:"name,omitempty"`
	Group        string  `json:"group,omitempty"`
	ReferenceRGB RGB     `json:"reference_linear_rgb"`
	MeasuredRGB  RGB     `json:"measured_rgb"`
	SigmaRGB     RGB     `json:"sigma_rgb,omitempty"`
	ClipRGB      [3]bool `json:"clip_rgb,omitempty"`
	MaskRGB      [3]bool `json:"mask_rgb,omitempty"`
}

// Chart is the only input consumed by the calibrator. It must not contain
// generator truth; acceptance fixtures keep Truth in the test/harness.
type Chart struct {
	SchemaVersion   string  `json:"schema_version,omitempty"`
	ValueSpace      string  `json:"value_space,omitempty"`
	BitDepth        int     `json:"bit_depth,omitempty"`
	BlackLevel      float64 `json:"black_level,omitempty"`
	SaturationLevel float64 `json:"saturation_level,omitempty"`
	Patches         []Patch `json:"patches"`
}

// Params are reported in the gauge where the geometric mean of WB is one.
type Params struct {
	Model           string  `json:"model"`
	Exposure        float64 `json:"exposure"`
	WhiteBalance    RGB     `json:"white_balance"`
	Gamma           RGB     `json:"channel_response_gamma"`
	EffectiveGain   RGB     `json:"effective_gain"`
	LogExposure     float64 `json:"log_exposure"`
	LogWhiteBalance RGB     `json:"log_white_balance"`
	LogGain         RGB     `json:"log_effective_gain"`
}

// ChannelDiagnostic describes whether one channel measurement can be fitted.
type ChannelDiagnostic struct {
	Channel          int     `json:"channel"`
	Used             bool    `json:"used"`
	Clipped          bool    `json:"clipped,omitempty"`
	LowSignal        bool    `json:"low_signal,omitempty"`
	RobustWeight     float64 `json:"robust_weight"`
	StandardResidual float64 `json:"standard_residual"`
	Predicted        float64 `json:"predicted"`
	Measured         float64 `json:"measured"`
	Reason           string  `json:"reason,omitempty"`
}

// PatchReport exposes squares that should not be trusted for correction.
type PatchReport struct {
	ID              string              `json:"id"`
	Name            string              `json:"name,omitempty"`
	Group           string              `json:"group,omitempty"`
	Status          string              `json:"status"` // reliable | downweighted | unreliable
	Reasons         []string            `json:"reasons,omitempty"`
	Channels        []ChannelDiagnostic `json:"channels"`
	Weight          float64             `json:"weight"`
	CorrectedRGB    RGB                 `json:"corrected_linear_rgb,omitempty"`
	ReferenceDeltaE float64             `json:"reference_delta_e2000,omitempty"`
}

// Coupling describes a statistically visible parameter dependency.
type Coupling struct {
	Parameters  [2]string `json:"parameters"`
	Correlation float64   `json:"correlation"`
	Strength    string    `json:"strength"`
	Message     string    `json:"message,omitempty"`
}

// FitDiagnostics contains numerical identifiability information.
type FitDiagnostics struct {
	Observations         int         `json:"observations"`
	WeightedObservations int         `json:"weighted_observations"`
	Parameters           int         `json:"parameters"`
	Iterations           int         `json:"iterations"`
	Converged            bool        `json:"converged"`
	ReducedChiSquare     float64     `json:"reduced_chi_square"`
	MatrixCondition      float64     `json:"matrix_condition_number"`
	SmallestEigenvalue   float64     `json:"smallest_eigenvalue"`
	Identifiable         bool        `json:"identifiable"`
	Couplings            []Coupling  `json:"couplings"`
	CovarianceLabels     []string    `json:"covariance_labels"`
	Covariance           [][]float64 `json:"covariance"`
	Correlation          [][]float64 `json:"correlation"`
	Notes                []string    `json:"notes,omitempty"`
}

// Report is the complete calibration result.
type Report struct {
	Params         Params         `json:"params"`
	Patches        []PatchReport  `json:"patches"`
	Diagnostics    FitDiagnostics `json:"diagnostics"`
	NaiveMeanAlign *ColorAccuracy `json:"naive_mean_align,omitempty"`
	JointFit       *ColorAccuracy `json:"joint_fit,omitempty"`
}

// ColorAccuracy summarizes held-out or named color-group errors.
type ColorAccuracy struct {
	Method     string   `json:"method"`
	PatchIDs   []string `json:"patch_ids"`
	MeanDeltaE float64  `json:"mean_delta_e2000"`
	P95DeltaE  float64  `json:"p95_delta_e2000"`
	MaxDeltaE  float64  `json:"max_delta_e2000"`
	SkinDeltaE float64  `json:"skin_mean_delta_e2000,omitempty"`
}
