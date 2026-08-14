package doctor

type State string

const (
	Pass State = "pass"
	Info State = "info"
	Fail State = "fail"
)

type Check struct {
	Name   string
	Detail string
	State  State
}

type Report struct {
	Checks  []Check
	Ready   bool
	Summary string
}
