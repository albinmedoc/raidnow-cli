package output

type ErrorCode string

type CLIError struct {
	Code    ErrorCode
	Message string
}

func NewError(code ErrorCode, message string) *CLIError {
	return &CLIError{Code: code, Message: message}
}

func (err *CLIError) Error() string {
	return err.Message
}

type SuccessEnvelope struct {
	OK       bool     `json:"ok"`
	Command  string   `json:"command"`
	Status   string   `json:"status"`
	Data     any      `json:"data"`
	Warnings []string `json:"warnings"`
}

type ErrorDetail struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type FailureEnvelope struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Status  string      `json:"status"`
	Error   ErrorDetail `json:"error"`
}

func NewSuccess(command, status string, data any, warnings []string) SuccessEnvelope {
	if warnings == nil {
		warnings = []string{}
	}
	return SuccessEnvelope{
		OK:       true,
		Command:  command,
		Status:   status,
		Data:     data,
		Warnings: warnings,
	}
}

func NewFailure(command, status string, err error, secrets ...string) FailureEnvelope {
	publicError := PublicError(err)
	return FailureEnvelope{
		OK:      false,
		Command: command,
		Status:  status,
		Error: ErrorDetail{
			Code:    publicError.Code,
			Message: Redact(publicError.Message, secrets...),
		},
	}
}
