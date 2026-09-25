package android

// AVDInfo is the read-only view of an AVD that doctor needs.
type AVDInfo struct {
	Name   string
	Config map[string]string
}

// ListAVDs reads every AVD under avdHome without running any command.
func ListAVDs(avdHome string) ([]AVDInfo, error) {
	all, err := (&Provider{AVDHome: avdHome}).avds()
	if err != nil {
		return nil, err
	}
	out := make([]AVDInfo, len(all))
	for i, a := range all {
		out[i] = AVDInfo{Name: a.Name, Config: a.Config}
	}
	return out, nil
}

// APILevel returns the AVD's API level ("33"), or "".
func APILevel(cfg map[string]string) string { return apiLevel(cfg) }

// Is16k reports whether the AVD uses a 16 KB page-size image, with a reason.
func Is16k(cfg map[string]string) (bool, string) { return is16k(cfg) }
