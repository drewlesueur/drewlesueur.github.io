#!/bin/bash
source ./touchcode.sh
rm -f /tmp/f && mkfifo /tmp/f
rm -f /tmp/o && mkfifo /tmp/o
# my version doesn't have the -e option

logs() {
    while true
    do
        read line </tmp/o
        echo "$line"
    done
}

logs &

handle() {
    echo $'HTTP/1.1 200 OK\r'
    read METHOD THEPATH REST

    THEFUNCNAME=$(echo "$THEPATH" | cut -c 2-)
    # echo $METHOD $THEPATH $REST | hexdump -c
    while read line
    do
        # echo "$line" | hexdump -c
        if { test "$METHOD" = "GET"; } && { test "$line" = $'\r'; }
        then
           break
        elif test "$METHOD" = "POST"
        then
            if echo "$line" | grep "Content-Length" > /dev/null
            then
                CONTENT_LENGTH=$(echo "$line" | awk '{print $2}' | sed 's/\s//g')
            elif test "$line" = $'\r'
            then
                BODY=$(head -c "$CONTENT_LENGTH") 
                break
            fi
        fi
    done
    FULLFUNCNAME="my${THEFUNCNAME}"
   
    "$FULLFUNCNAME"
    echo "func name: $THEFUNCNAME $FULLFUNCNAME" >/tmp/o
}

# netcat -lp 8080 -e handle.sh
while true
do
    cat /tmp/f | handle | netcat -l 1234 > /tmp/f
done
