package javaio

type Serializable struct {
	Value interface{} `javaio:"-"`
}

func (Serializable) ClassName() string {
	return "java.io.Serializable"
}

func (Serializable) SerialVersionUID() int64 {
	return 1196656838076753133
}
