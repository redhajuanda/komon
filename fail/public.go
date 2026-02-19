package fail

// Failure represents a well-known, public-safe error definition
// that can be returned to end users (e.g. in HTTP responses).
// Register these once at startup using Register.
type Failure struct {
	// Code is a unique application-level error code (e.g. "400001")
	Code string
	// Message is the human-readable message safe to expose to end users
	Message string
	// HTTPStatus is the HTTP status code to use when returning this error
	HTTPStatus int
}

func (p *Failure) TemperMessage(message string) *Failure {
	p.Message = message
	return p
}

func (p *Failure) TemperHTTPStatus(httpStatus int) *Failure {
	p.HTTPStatus = httpStatus
	return p
}

func (p *Failure) TemperCode(code string) *Failure {
	p.Code = code
	return p
}

var (
	// Built-in failures (public-safe). These are registered automatically.
	ErrInternalServer = Register("500000", "An internal server error occurred", 500)
	ErrBadRequest     = Register("400000", "Your request is invalid", 400)
	ErrUnauthorized   = Register("401000", "You are not authorized to access this resource", 401)
	ErrNotFound       = Register("404000", "Your requested resource is not found", 404)
	ErrForbidden      = Register("403000", "You are forbidden to access this resource", 403)
	ErrUnprocessable  = Register("422000", "Your request is unprocessable", 422)
	ErrConflict       = Register("409000", "Your request is conflicting with the current state of the resource", 409)
	ErrTooManyRequest = Register("429000", "You have sent too many requests", 429)
)

// Register registers a new Failure (public-safe) and returns it.
//
// Usage:
//
//	var ErrUserNotFound = fail.Register("404001", "User not found", 404)
func Register(code, message string, httpStatus int) *Failure {
	return &Failure{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}
