
# 标题一

## 标题二

### 标题三

#### 标题四

##### 标题五

###### 标题六

Lorem ipsum dolor sit amet, consectetur adipiscing elit.

天下之患，最不可为者，名为治平无事，而其实有不测之忧。

> 君子博学而日参省乎己，则知明而行无过矣。

```javascript
const greet = (name) => `hello ${name}`;
```

* HTML 超文本传输协议
* CSS 层叠样式表
* JavaScript 脚本

$$R∪S=\{t|t∈R∨t∈S\}$$

```mermaid
graph TD
    %% ── 入口 ──────────────────────────────────────────────
    main["main()\n检查 APP_WORKER_MODE"]

    main -->|"== '1'"| runWorker
    main -->|"其他"| runMaster

    %% ── MASTER 侧 ─────────────────────────────────────────
    subgraph MASTER["MASTER 进程"]
        runMaster["runMaster()\nnet.Listen :8080\n监听信号"]
        mustSpawn["mustSpawn(ln)"]
        spawnWorker["spawnWorker(ln)\n复制 listener fd\n创建 ready pipe\nexec.Command(self)\nExtraFiles=[lnFile, rw]"]
        workerStruct["worker{cmd, done chan}"]
        drainWorker["drainWorker(w)\n→ SIGUSR1\n等待退出 / Kill"]
        stopWorker["stopWorker(w)\n→ SIGTERM\n等待退出 / Kill"]
    end

    runMaster --> mustSpawn
    mustSpawn --> spawnWorker
    spawnWorker --> workerStruct
    spawnWorker -->|"就绪超时/未就绪"| Kill1["Kill()"]

    %% 信号分支
    runMaster -->|"SIGHUP"| mustSpawn
    runMaster -->|"SIGHUP 完成"| drainWorker
    runMaster -->|"SIGINT/SIGTERM"| stopWorker
    runMaster -->|"worker.done 关闭"| mustSpawn

    %% ── IPC 通道 ──────────────────────────────────────────
    subgraph IPC["进程间通信（IPC）"]
        lnFD["继承 fd 3\nTCP Listener"]
        readyPipe["ready pipe\nfd 4 写端→子进程\n读端→master"]
        envVars["环境变量\nAPP_WORKER_MODE=1\nAPP_LISTENER_FD=3\nAPP_READY_FD=4"]
        readyMsg["readyMessage='READY'\\n"]
    end

    spawnWorker -->|"ExtraFiles[0]"| lnFD
    spawnWorker -->|"ExtraFiles[1]"| readyPipe
    spawnWorker -->|"cmd.Env"| envVars

    %% ── WORKER 侧 ─────────────────────────────────────────
    subgraph WORKER["WORKER 子进程"]
        runWorker["runWorker()"]
        inheritedListener["inheritedListener()\n读 APP_LISTENER_FD\nos.NewFile → net.FileListener"]
        fiberApp["fiber.New()\nGET / → SendString pid"]
        onListen["Hooks.OnListen\n→ signalReady()"]
        signalReady["signalReady()\n读 APP_READY_FD\nfmt.Fprintln 'READY'"]
        sigGo["goroutine\nSIGUSR1/SIGTERM/SIGINT\n→ app.ShutdownWithTimeout"]
    end

    runWorker --> inheritedListener
    inheritedListener -->|"从 fd 3 还原"| lnFD
    runWorker --> fiberApp
    fiberApp --> onListen
    onListen --> signalReady
    signalReady -->|"写入"| readyPipe
    readyPipe -->|"master 读到 READY"| readyMsg
    readyMsg -->|"就绪确认"| workerStruct
    runWorker --> sigGo

    %% 信号从 master 到 worker
    drainWorker -->|"SIGUSR1"| sigGo
    stopWorker  -->|"SIGTERM"| sigGo
    sigGo       -->|"ShutdownWithTimeout"| fiberApp
```