package proxy

// Simplifier is the interface for HTML simplifiers (HTML 3.2 vs 4.01 etc.)
type Simplifier interface {
	Simplify(inputHTML string, pageURL string, debugMode bool) (string, error)
}
