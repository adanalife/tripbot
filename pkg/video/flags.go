package video

//TODO: this really shouldnt live in the video pkg,
// but there was an import cycle

func ShowFlag() {
	//TODO: this should trigger when a state change event fires instead of every time we run this
	updateFlagFile()
	// actually display the flag
	background.FlagImage.ShowFor("", 10*time.Second)
}

// updateFlagFile replaces the current flag image with the current state flag
func updateFlagFile() {
	if helpers.FileExists(background.FlagImageFile) {
		if config.Verbose {
			log.Printf("removing %s because it already exists", background.FlagImageFile)
		}
		// remove the existing flag file
		err := os.Remove(background.FlagImageFile)
		if err != nil {
			terrors.Log(err, "error removing old flag image")
		}
	}

	// this is the image we should be showing
	newFlagFile := flagSourceFile(CurrentlyPlaying.State)

	// copy the image to the live location
	err := os.Symlink(newFlagFile, background.FlagImageFile)
	if err != nil {
		terrors.Log(err, "error creating new flag image")
	}
}

// flagSourceFile returns the full path to a flag image file
func flagSourceFile(state string) string {
	// make it lowercase
	state = strings.ToLower(state)
	fileName := fmt.Sprintf("%s.jpg", state)
	return path.Join(helpers.ProjectRoot(), "assets/flags/small", fileName)
}
