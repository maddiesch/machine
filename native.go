package machine

var (
	nativeFunctionNames = []string{
		"_delete",
	}

	reservedWords = []string{
		"const",
		"true",
		"false",
	}
)

func contains(set []string, word string) bool {
	for _, in := range set {
		if word == in {
			return true
		}
	}
	return false
}
