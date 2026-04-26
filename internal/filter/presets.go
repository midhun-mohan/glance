package filter

type PresetManager struct {
	presets map[string]string
}

func NewPresetManager(presets map[string]string) *PresetManager {
	if presets == nil {
		presets = map[string]string{}
	}
	return &PresetManager{presets: presets}
}

func (pm *PresetManager) Get(name string) (FilterSet, bool) {
	expr, ok := pm.presets[name]
	if !ok {
		return FilterSet{}, false
	}
	return Parse(expr), true
}

func (pm *PresetManager) List() map[string]string {
	return pm.presets
}
