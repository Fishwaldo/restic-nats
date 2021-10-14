package rns

//NatsCommand is the Command the packet represents.
type NatsCommand string

const (
	//NatsOpenCmd - Open a Repository
	NatsOpenCmd NatsCommand = "open"
	//NatsStatCmd - Stat a file on the Repository
	NatsStatCmd NatsCommand = "stat"
	//NatsMkdirCmd - Mkdir on the Repository
	NatsMkdirCmd NatsCommand = "mkdir"
	//NatsSaveCmd - Save a file to the respository
	NatsSaveCmd NatsCommand = "save"
	//NatsListCmd - List files in a Repository
	NatsListCmd NatsCommand = "list"
	//NatsLoadCmd - Load a file from a respository
	NatsLoadCmd NatsCommand = "load"
	//NatsRemoveCmd - Remove a file from a Repository
	NatsRemoveCmd NatsCommand = "remove"
	//NatsCloseCmd - Close a Repository
	NatsCloseCmd NatsCommand = "close"
)

//OpenRepoOp - Open a Repository
type OpenRepoOp struct {
	//Bucket the Repository to open
	Bucket string `json:"bucket"`
	//Client Name
	Client string `json:"client"`
	//Host name
	Hostname string `json:"hostname"`
}

//OpenRepoResult - The result of Opening a Repository
type OpenRepoResult struct {
	// Ok - If the Repository is successfully opened
	Ok bool `json:"ok"`
	// Err, if any
	Err error `json:"err"`
	//ClientID - The ClientID
	ClientID string `json:"clientid"`
}

//StatOp - Stat a file in the repository Bucket
type StatOp struct {
	//Directory the Directory the file lives in
	Directory string `json:"directory"`
	//Filename the Filename to Stat
	Filename string `json:"filename"`
}

//StatResult - the result of Stating a file
type StatResult struct {
	//Ok - If the Stat Command was successful
	Ok bool `json:"ok"`
	//Size - The Size of the file
	Size int64 `json:"size"`
	//Name - The Name of the file
	Name string `json:"name"`
	//Err - Error
	Err error
}

//MkdirOp - Make a Directory in the Repository
type MkdirOp struct {
	//Dir the name of the directory to create
	Dir string `json:"dir"`
}

//MkdirResult - THe result of Making a Directory
type MkdirResult struct {
	//Ok - if the Mkdir Command was successful
	Ok bool `json:"ok"`
	//Err, if any
	Err error
}

//SaveOp - Save a file to the respository
type SaveOp struct {
	//Dir - The Directory to save the file in
	Dir string `json:"dir"`
	//Name - The Name of the file to save
	Name string `json:"name"`
	//FileSize - the size of the entire file
	Filesize int `json:"size"`
	//Data - The actual file data
	Data []byte `json:"data"`
}

//SaveResult - The result of saving a file
type SaveResult struct {
	//Ok - of the save command was successful
	Ok bool `json:"ok"`
	//Err - Error, if any
	Err error
}

//ListOp - List files in a directory (and optionally, subdirectories)
type ListOp struct {
	//Basedir - the Base Directory to list
	BaseDir string `json:"base_dir"`
	//Subdir - If we would recurse into subsdirectories
	Recurse bool `json:"recurse"`
}

//FileInfo - File Information returned for each file in a ListOp Command
type FileInfo struct {
	//Name - The name fo the file
	Name string `json:"name"`
	//Size - The size of the file
	Size int64 `json:"size"`
}

//ListResult - The result of listing files
type ListResult struct {
	//Ok - If the command was succesful
	Ok bool `json:"ok"`
	//FI - Slice of FileInfo for all files found
	FI []FileInfo `json:"fi"`
	//Err - Error, if any
	Err error
}

//LoadOp - Read a file from a respository
type LoadOp struct {
	//Dir - The Directory to read the file from
	Dir string `json:"dir"`
	//Name - The name of the file to Load
	Name string `json:"name"`
	//Length - How much data to load from the file
	Length int `json:"length"`
	//Offset - The offset where we should start reading the file from
	Offset int64 `json:"offset"`
}

//LoadResult - The result of loading a file
type LoadResult struct {
	//Ok - if the command was successful
	Ok bool `json:"ok"`
	//Data - slice of bytes contianing the file contents
	Data []byte `json:"data"`
	//Err - Error, if any
	Err error
}

//RemoveOp - Remove a file from the Repository
type RemoveOp struct {
	//Dir - The Name of the directory where the file resides
	Dir string `json:"dir"`
	//Name - Name of the file to remove
	Name string `json:"name"`
}

//RemoveResult - The result of removing a file
type RemoveResult struct {
	//Ok - if the command was scuccessful
	Ok bool `json:"ok"`
	//Err - Error, if any
	Err error
}

//CloseOp - Close a Repository
type CloseOp struct {
}

//CloseResult - The result of Closing a Repository
type CloseResult struct {
	//Ok - if the command was scuccessful
	Ok bool `json:"ok"`
	//Err - Error, if any
	Err error
}
