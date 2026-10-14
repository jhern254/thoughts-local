package modelassets

// DownloadProgress contains only byte counts, never URLs, paths or file content.
type DownloadProgress struct {
	DownloadedBytes int64
	TotalBytes      int64
}

// ModelDescription is display metadata, not evidence of installation validity.
type ModelDescription struct {
	ID                ModelID
	DisplayName       string
	Revision          string
	ExpectedSizeBytes int64
	Attribution       string
}

func DescribeModel(modelID ModelID) (ModelDescription, error) {
	manifest := moonshineSmallStreamingManifest()
	if modelID != manifest.ID {
		return ModelDescription{}, ErrUnknownModel
	}
	return describeModelManifest(manifest), nil
}
func describeModelManifest(manifest modelManifest) ModelDescription {
	description := ModelDescription{
		ID:          manifest.ID,
		DisplayName: manifest.DisplayName,
		Revision:    manifest.Revision,
		Attribution: manifest.Attribution,
	}
	for _, modelFile := range manifest.Files {
		description.ExpectedSizeBytes += modelFile.ExpectedSizeBytes
	}
	return description
}
