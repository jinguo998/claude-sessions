package ports

type Clipboard interface {
	Copy(text string) error
}

type Editor interface {
	Open(filePath string) error
}
