package locale

// Template-argument names used in locale.Tr calls. Each matches a
// {{.Name}} placeholder in one or more catalog entries. Kept as
// constants so typos are a compile error, not a runtime "<no value>".
const (
	ArgAddress = "Address"
	ArgBody    = "Body"
	ArgError   = "Error"
	ArgLabel   = "Label"
	ArgN       = "N"
	ArgName    = "Name"
	ArgPrefix  = "Prefix"
	ArgReason  = "Reason"
	ArgRegion  = "Region"
)
