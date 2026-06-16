package layer

// InstallMode controls how files are installed.
type InstallMode int

const (
	// InstallSymlink creates symlinks from $HOME to layer files.
	InstallSymlink InstallMode = iota
	// InstallCopy copies layer files to $HOME.
	InstallCopy
	// InstallDryRun prints what would be done without doing it.
	InstallDryRun
)
