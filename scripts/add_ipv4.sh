#!/bin/bash

# ./add_ipv4.sh 2.2.2.1 2.2.2.100 eth1
# 定义起始和结束IP地址范围
start_ip=$1
end_ip=$2
iface=$3

# 将IP地址转换为整数表示方便操作
ip_to_int() {
    local IFS='.'
    read -r i1 i2 i3 i4 <<< "$1"
    echo $(( (i1<<24) + (i2<<16) + (i3<<8) + i4 ))
}

# 将整数表示的IP地址转换为正常格式
int_to_ip() {
    local ip=$1
    echo $(( (ip>>24) & 255 )).$(( (ip>>16) & 255 )).$(( (ip>>8) & 255 )).$(( ip & 255 ))
}

# 获取起始和结束IP地址的整数表示
start_int=$(ip_to_int "$start_ip")
end_int=$(ip_to_int "$end_ip")

# 循环遍历IP地址范围
for ((ip_int=start_int; ip_int<=end_int; ip_int++))
do
    ip=$(int_to_ip $ip_int)
    # 检查IP地址是否已存在
    ip addr add "$ip/24" dev $iface
    echo "\"$ip\","
done
