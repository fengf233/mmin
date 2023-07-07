#!/bin/bash
while true
do
    clear # 清空屏幕
    echo "TCP状态统计："
    ss -ant  | awk 'NR>1 {++s[$1]} END {for(k in s) print k,s[k]}' # 统计TCP连接状态
    sleep 1 # 1秒后刷新
done