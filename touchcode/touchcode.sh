myfiles() {
    echo $'Content-Type: text/html\r'
    echo $'\r'
    ls
}

myupdir() {
    echo $'Content-Type: text/html\r'
    echo $'\r'
    cd ..
    ls
}

mygotodir() {
    cd $1
    ls
}


mygitstatus() {
    echo $'Content-Type: text/html\r'
    echo $'\r'
    git status
}
