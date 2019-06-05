package javaio

type String struct {
	Value string `javaio:"-"`
}

func (String) ClassName() string {
	return "java.lang.String"
}

func (String) SerialVersionUID() int64 {
	return -6849794470754667710
}
