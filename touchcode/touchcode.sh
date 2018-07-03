myfiles() {
    echo $'Content-Type: text/html\r'
    echo $'\r'
    ls
}

myupdir() {
    cd ..
    ls
}

mygotodir() {
    cd $1
}
