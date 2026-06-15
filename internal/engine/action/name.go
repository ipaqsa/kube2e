package action

// Name identifies the kind of action recorded in a report.
type Name string

const (
	// NameEnsure applies an object to the cluster with Server-Side Apply.
	NameEnsure Name = "ensure"
	// NamePatch applies RFC 6902 patches to a live object.
	NamePatch Name = "patch"
	// NameWait polls an object until its conditions hold.
	NameWait Name = "wait"
	// NameAssert checks JQ conditions against an object once.
	NameAssert Name = "assert"
	// NameLogs polls pod logs until they match.
	NameLogs Name = "logs"
	// NameExec runs a command inside a pod.
	NameExec Name = "exec"
	// NameDelete removes an object from the cluster.
	NameDelete Name = "delete"
)
