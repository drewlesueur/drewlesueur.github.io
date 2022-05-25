package main

import (
	"fmt"
	// "log"
	"strings"
)

func runApplyDiff(a, b string) string {
	// trim leading and trailing lines and run diff
	aLines := strings.Split(a, "\n")
	a = strings.Join(aLines[1:len(aLines)-1], "\n")

	bLines := strings.Split(b, "\n")
	b = strings.Join(bLines[1:len(bLines)-1], "\n")

	// log.Printf("a: %s", a)
	// log.Printf("b: %s", b)
	ret, err := applyDiff(a, b)
	if err != nil {
		panic(err)
	}
	return ret
}

func ExampleApplyDiff_addAtEnd() {
	fmt.Println(runApplyDiff(`
Hello
    `, `
@@ -2 @@
+World
    `))

	// Output:
	// Hello
	// World
}

func ExampleApplyDiff_addAtBeginning() {
	fmt.Println(runApplyDiff(`
Hello
World
    `, `
@@ -1 @@
+another
    `))

	// Output:
	// another
	// Hello
	// World
}

func ExampleApplyDiff_removeandAddAtBeginning() {
	fmt.Println(runApplyDiff(`
Hello
World
    `, `
@@ -1 @@
-Hello
+Friends
    `))

	// Output:
	// Friends
	// World
}

func ExampleApplyDiff_removeAtBeginning() {
	fmt.Println(runApplyDiff(`
Hello
World
    `, `
@@ -1 @@
-Hello
    `))

	// Output:
	// World
}

func ExampleApplyDiff_removeAtEnd() {
	fmt.Println(runApplyDiff(`
Hello
World
    `, `
@@ -2 @@
-World
    `))

	// Output:
	// Hello
}

func ExampleApplyDiff_removeInMiddle() {
	fmt.Println(runApplyDiff(`
Hello
World
Stuff
    `, `
@@ -2 @@
-World
    `))

	// Output:
	// Hello
	// Stuff
}

func ExampleApplyDiff_TwoChunksRemoveAdd() {
	fmt.Println(runApplyDiff(`
Hello
1
2
3
4

Other
a
b
c
d
    `, `
@@ -3 @@
-2
-3
+22
+33
+44
@@ -9 @@
-b
-c
+bb
+cc
+dd
    `))

	// Output:
	// Hello
	// 1
	// 22
	// 33
	// 44
	// 4
	//
	// Other
	// a
	// bb
	// cc
	// dd
	// d
}
