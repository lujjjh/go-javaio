# go-javaio

## Get Started

```sh
go get github.com/lujjjh/go-javaio/...
```

`struct`s to be serialized must implements `ClassName() string` method
which returns the Java class name.

Optionally, `SerialVersionUID() int64` may be implemented if you want to
specify the serialVersionUID of the class.

For example, define a `List` struct:

```
type List struct {
	Value int32
	Next  *List
}

func (*List) ClassName() string {
	return "List"
}

func (*List) SerialVersionUID() int64 {
	return 1
}
```

Create `List` instances and serialize them:

```go
list2 := &List{
    Value: 19,
}
list1 := &List{
    Value: 17,
    Next:  list2,
}

var buf bytes.Buffer
w, _ := NewStreamWriter(&buf)
w.writeObject(list1)
w.writeObject(list2)
```

The resulting buffer contains:

```go
00000000  ac ed 00 05 73 72 00 04  4c 69 73 74 00 00 00 00  |....sr..List....|
00000010  00 00 00 01 02 00 02 49  00 05 76 61 6c 75 65 4c  |.......I..valueL|
00000020  00 04 6e 65 78 74 74 00  06 4c 4c 69 73 74 3b 78  |..nextt..LList;x|
00000030  70 00 00 00 11 73 71 00  7e 00 00 00 00 00 13 70  |p....sq.~......p|
00000040  71 00 7e 00 03                                    |q.~..|
```

## Todo List

- [x] Serialization
- [ ] Deserialization
