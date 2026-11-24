package main

// HashFile builds a manifest with DefaultOptions.
func HashFile(path string) (*Manifest, error) {
	archive, err := OpenArchive(path, DefaultOptions())
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	return archive.Manifest, nil
}

// DiffFiles compares two paths with DefaultOptions.
func DiffFiles(originalPath, copyPath string) (*Report, error) {
	return CompareFiles(originalPath, copyPath, DefaultOptions())
}
