- [工具介绍](#工具介绍)
- [快速使用](#快速使用)
  - [命令行参数说明](#命令行参数说明)
  - [结果说明](#结果说明)
- [配置说明](#配置说明)
- [测试案例](#测试案例)
  - [QPS和吞吐](#qps和吞吐)
  - [并发连接数](#并发连接数)
  - [新建连接速率](#新建连接速率)
  - [多用户并发](#多用户并发)
- [免责声明](#免责声明)
- [参考](#参考)

---
## 工具介绍

mmin是一个go实现的基于TCP的压测工具，目的是为了模拟测试应用层设备（web服务器，waf，应用层防火墙）等性能，包括QPS/TPS，应用层吞吐，新建连接，并发连接等，得益于go的并发管理，可以充分运用客户端的资源。

- 类似于jmeter线程组，可以在一个协程中发送多个HTTP请求
- 支持远程控制多台设备一起压测

以后可能支持:

- http2.0
- 基于国密的HTTPS测试
- http变量替换


> 目前只支持http压测，但是容易拓展其他TCP私有协议


## 快速使用
```
./mmin -u http://www.test.com/ -c 100 -t 20
```

> 注意:当设置线程数过大时,需要查看进程文件句柄是否有限制,ulimit -n查看,如果-c大于ulimit -n,可能会报错open file too many

### 命令行参数说明

```shell
Usage of ./mmin:
  -H value
        自定义头:-H 'Content-Type: application/json'
  -P string
        -S下生效,指定监听端口 (default "8888")
  -R int
        限制RPS发送请求速率,默认不限制
  -S    以服务端的方式运行,-P指定监听端口
  -c int
        请求线程数,默认100 (default 100)
  -conf string
        指定运行的yaml文件,其他选项会失效,优先执行yaml文件
  -data string
        发送body:-data 'a=1&b=2'
  -debug
        是否打开打印调试信息
  -k int
        设置每个TCP最多发送多少请求,默认100 (default 100)
  -m string
        请求方法 (default "GET")
  -t int
        运行时间,默认10s (default 10)
  -u string
        URL地址:http://test.com:8080/test
```

### 结果说明

```shell
Creat TCP conns Start:
Time       ConnCount   
---------- ------------
1          92          
Creat TCP conns: 100

Time       Success      Rate         ReqTime      Send         Receive      Status              
---------- ------------ ------------ ------------ ------------ ------------ --------------------
1.0104218  8000         8000         10.916383    3.566506     52.043415    [200]:8000          
2.0141416  15077        7077         12.503663    3.346634     49.08017     [200]:15077         
3.075825   19722        4645         18.484386    2.8569558    41.97926     [200]:19722         
4.077483   27194        7472         11.107747    2.9669635    43.638382    [200]:27194         
5.0766826  35097        7903         10.674035    3.070042     45.195717    [200]:35097         
6.081588   41899        6802         12.275972    3.0591137    45.05245     [200]:41899         
7.0822926  50969        9070         10.218876    3.1936321    47.045       [200]:50969         
8.082782   57933        6964         12.646858    3.178943     46.84416     [200]:57933         
9.087093   65644        7711         11.287118    3.2038012    47.218437    [200]:65644         

Result     Statistics                                                                           
---------- -------------------------------------------------------------------------------------
RunTime:   10.000029 s                                                                          
Success:   75241                                                                                
AvgRate:   7524.078125 Req/s                                                                    
ReqTime:   11.516679 ms                                                                         
Send:      3.333734 Mbps                                                                        
Receive:   49.140011 Mbps                                                                       
Status:    [200]:75241                                                                          
ReqTime Quantile: 50: 7.747041 75: 8.903125 90: 10.270083 95: 12.527833 99: 124.939167 
```
输出分为三个部分，第一个部分为创建TCP的结果，第二个部分为实时每秒的统计，第三个部分为总的统计结果
```
RunTime 运行时间
Success 发送成功数量
AvgRate 平均QPS
ReqTime 平均请求响应时间,从发送到响应的时间
Send    发送的应用层吞吐Mbps
Receive 接收的应用层吞吐Mbps
Status  响应码统计
ReqTime Quantile: 请求响应时间分位统计
```

## 配置说明

除了类似于ab的运行方式，还支持运行conf的方式，当使用-conf后，会根据指定的yaml运行压测，配置说明如下

```yaml

RunTime: 5                          #总体运行时间           
Debug: true                         #是否开启debug打印
RemoteServer: {                     #指定远程服务器运行对应的group,如果为空,表示本地执行
  "test_server1:8888":["group1"],   #远程服务器地址,以及远程服务器要运行的groupName,支持多个
  "test_server2:8888":["group2"],
}

TcpGroups:              
- Name: group1                      #group标志符,用于远程执行
  MaxTcpConnPerIP: 10000            #每个源IP创建最大的TCP连接数
  TcpCreatThread: 1                 #初始化创建TCP池的线程,一般为1就行,只是测大并发时可以调高
  TcpConnThread: 10                 #循环生产TCP池的线程,就是当MaxReqest满足关闭TCP后,补充创建TCP的线程,长连接的情况设置ReqThread/MaxReqest就差不多了
  TcpCreatRate: 0                   #初始化创建TCP的速率,0为不限制
  SrcIP: ["2.0.0.19","2.0.0.100" ]  #源IP,为[]表示使用默认IP
  MaxQps: 500                       #发送http请求最大QPS 
  Dst: 2.0.0.67:80                  #TCP目的地址,只支持ip
  ReqThread: 10                     #发送http请求的线程数
  MaxReqest: 100                    #每个TCP连接最多发送多少连接
  IsHttps: false                    #是否是https
  SendHttp: ["test1","test2"]       #每个TCP连接中循环发送的http请求
- Name: group2                      #可以配置多个组
  MaxTcpConnPerIP: 10000
  TcpCreatThread: 1            
  TcpConnThread: 10                 
  SrcIP: ["2.0.0.19","2.0.0.100" ]  
  MaxQps: 500                       
  Dst: 2.0.0.67:80                  
  ReqThread: 10                     
  MaxReqest: 100                    
  IsHttps: false                    
  SendHttp: ["test2"]               

HTTPConfs:                          #定义发送的http请求
- Name: test1                       #http请求标志符
  Proto: HTTP/1.1                   #http协议,HTTP/1.0,HTTP/1.1
  Method: GET                       #请求方法
  URI: http://2.0.0.67?a=0          #URL
  Header: {                         #header,键值对方式
    "test":"faf"
  }
  Body: ""                          #body内容,字符串方式
- Name: test2
  Proto: HTTP/1.1
  Method: POST
  URI: http://2.0.0.67?a=1
  Header: {
    "test":"faf"
  }
  Body: "a=b&&b=c"
```

## 测试案例

### QPS和吞吐

QPS表示每秒处理的HTTP请求数

如果是单个请求,直接使用命令行执行就行

```shell
./mmin -u http://www.test.com/ -c 100 -t 20
```
-c对应ReqThread（发送http请求的线程数），越大压测力度越大

使用配置的方式

```yaml
RunTime: 20                                    
Debug: false

TcpGroups: 
- Name: group1                      
  MaxTcpConnPerIP: 1000      #由于测试qps,一般是长连接,所以tcp可以不用设置太多,比ReqThread多一点就行
  TcpCreatThread: 1          #由于tcp较少,初始化1000个tcp只需要1个线程就够用了     
  TcpConnThread: 10          #由于测试qps,一般是长连接,回收TCP连接线程10个就够了      
  TcpCreatRate: 0            #1000个tcp,不用限制速率          
  SrcIP: []                  #源ip默认就行
  MaxQps: 0                  #设置0不限速,如果需要测试被压测机在某个QPS下的情况,可以设置限速                
  Dst: 2.0.0.67:80           #TCP目的地址,只支持ip         
  ReqThread: 1000            #请求线程, 越大压测力度越大                   
  MaxReqest: 100             #nginx默认长连接发送100个就会自动断开,一般默认100                    
  IsHttps: false             #不是https                   
  SendHttp: ["test1"]        #发送的http   

HTTPConfs:
- Name: test2
  Proto: HTTP/1.1
  Method: POST
  URI: http://2.0.0.67?a=1
  Header: {
    "test":"faf"
  }
  Body: "a=b&&b=c"

```
然后
```
./mmin -conf test.yaml
```

### 并发连接数

并发连接数主要是测试在创建几百万的TCP连接下，以一个新建速率发送HTTP请求，达到边建边拆，维持几百万的TCP连接，与一般的并发测试不同

要达到几百万的TCP连接，需要修改设置一些系统参数，如下

```
fs.nr_open                        #进程最大文件数
fs.file-max                       #系统最大文件数
nofile（soft 和 hard）ulimit –n    #进程最大文件数
#soft nofile 可以按用户来配置，而 fs.nr_open 所有用户只能配一个
net.ipv4.ip_local_port_range      #端口范围
```
一般来说，这样配置就行，推荐文章：https://mp.weixin.qq.com/s/GBn94vdL4xUL80WYrGdUWQ?from_wecom=1
```
# vi /etc/sysctl.conf
fs.nr_open=1100000  //要比 hard nofile 大一点
fs.file-max=1100000 //多留点buffer
# sysctl -p
# vi /etc/security/limits.conf
*  soft  nofile  1000000
*  hard  nofile  1000000
```
实测每个mmin进程最多70w个连接，多了就连接上不去了，所以建议多开几个窗口运行-S模式来并发几百万的连接

nginx需要设置
worker_connections 每个work可以建立的连接数
由于客户端会等待发送，需要设置nginx超时时间
client_body_timeout  500;
client_header_timeout  500;
send_timeout  500;
测试配置

```yaml
RunTime: 20                                    
Debug: false
RemoteServer: {                     
  "test_server1:8888":["group1"],   #server1创建50w个
  "test_server2:8888":["group2"],   #server2创建50w个
}

TcpGroups: 
- Name: group1                      
  MaxTcpConnPerIP: 50000      #每个srcip的TCP连接数,10个srcip就是50w
  TcpCreatThread: 10          #由于tcp较多,初始化50w个tcp可以调高一些   
  TcpConnThread: 10           #由于以MaxQps去边建边拆维持tcp连接，所以可以适当调高一点      
  TcpCreatRate: 100000        #50w个tcp,可以限制一定速率         
  SrcIP: [                    #源ip要10个，需要自己先把网卡的ip设置上,ip addr add 2.0.0.1/24 dev eth1
"2.0.0.1","2.0.0.2".."2.0.0.10"
  ]                  
  MaxQps: 10000               #以10000的速率维持               
  Dst: 2.0.0.67:80            #TCP目的地址,只支持ip         
  ReqThread: 100              #10000的速率100个够了                  
  MaxReqest: 1                #由于需要边建边拆，所以发送1个http请求就断开tcp                  
  IsHttps: false              #不是https                   
  SendHttp: ["test1"]         #发送的http   
- Name: group2                #类似group1,不过srcip建议分开
...
HTTPConfs:
- Name: test2
  Proto: HTTP/1.0             #设置http1.0,使服务端断开
  Method: GET
  URI: http://2.0.0.67
  Header: {
    "Connection":"close"      #或者设置close，使服务端断开连接
  }
  Body: ""
```
运行
```shell
server1:
./mmin -S
server2:
./mmin -S
server3:
./mmin -conf test.yaml
```

### 新建连接速率

TCP新建速率是衡量被测设备处理TCP建连断连的速率，对于应用层设备，新建连接速率也需要处理http请求，即每个tcp发送一个http请求就断开

配置如下:
```yaml
RunTime: 20                                    
Debug: false

TcpGroups: 
- Name: group1                      
  MaxTcpConnPerIP: 10000      #每个srcip的TCP连接数,新建连接就是不断建不断拆,这里总连接数可以与预估新建速率相仿,比如新建速率大概5w,有5个srcip,每个ip就10000
  TcpCreatThread: 10          #由于tcp较多,可以调高点 
  TcpConnThread: 1000         #由于新建连接就是不断建不断拆,所以可以与ReqThread接近,如果性能没有上去再调高     
  TcpCreatRate: 100000        #可以限制一定速率         
  SrcIP: [                    #源ip要5个，需要自己先把网卡的ip设置上,ip addr add 2.0.0.1/24 dev eth1
"2.0.0.1","2.0.0.2".."2.0.0.10"
  ]                           #为什么要多个ip,因为调用了conn.close,所以客户端也会有大量timewait,多点ip可以分摊些复用timewait的时间
  MaxQps: 50000               #预计50000的新建               
  Dst: 2.0.0.67:80            #TCP目的地址,只支持ip         
  ReqThread: 1000             #速率上不去可以调高                
  MaxReqest: 1                #由于需要边建边拆，所以发送1个http请求就断开tcp                  
  IsHttps: false              #不是https                   
  SendHttp: ["test1"]         #发送的http   

HTTPConfs:
- Name: test2
  Proto: HTTP/1.0             #设置http1.0,使服务端断开
  Method: GET
  URI: http://2.0.0.67
  Header: {
    "Connection":"close"      #或者设置close，使服务端断开连接
  }
  Body: ""
```

### 多用户并发

也可以测试多用户并发的场景,比如5w个用户并发

```yaml
RunTime: 20                                    
Debug: false

TcpGroups: 
- Name: group1                      
  MaxTcpConnPerIP: 50000     #5w个用户并发,可能需要5w个TCP
  TcpCreatThread: 10         #可以调高一点     
  TcpConnThread: 1000        #可以调高一些     
  TcpCreatRate: 0            #不用限制速率          
  SrcIP: []                  #源ip默认就行,不够可以加ip
  MaxQps: 0                  #设置0不限速,如果需要测试被压测机在某个QPS下的情况,可以设置限速                
  Dst: 2.0.0.67:80           #TCP目的地址,只支持ip         
  ReqThread: 50000           #模拟5w个用户                  
  MaxReqest: 100             #nginx默认长连接发送100个就会自动断开,一般默认100                    
  IsHttps: false             #不是https                   
  SendHttp: ["test1"]        #发送的http   

HTTPConfs:
- Name: test2
  Proto: HTTP/1.1
  Method: POST
  URI: http://2.0.0.67?a=1
  Header: {
    "test":"faf"
  }
  Body: "a=b&&b=c"

```

## 免责声明
本项目仅用于学习测试，请勿利用文章内的相关工具与技术从事非法测试，如因此产生的一切不良后果与本项目无关，用户承担因使用此工具而导致的所有法律和相关责任！作者不承担任何法律责任！


## 参考

https://github.com/link1st/go-stress-testing

https://github.com/six-ddc/plow