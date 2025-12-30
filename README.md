# agent

<h2>项目介绍</h2>


用于本人对 agent 应用开发的研究学习，该项目基于字节开源的 eino 框架。

目前已实现：

1. [魔改版 react agent](docs/react.md)  ｜ [代码](pkg/t_eino/react.go): 
   1. batch 节点完全阻塞 chatModel 输出流，判断是否有 function call ，解决原 react 只判断第一个 message 片段有无 function call 的尴尬短板。
   2. 采用 callback 机制实时读取 react 的输出，而无需等待流到 end 节点，同时规避了上一点的完全阻塞问题。
2. agui 协议适配 | [代码]():
   1. 借助

todo:

1. skill：一个粒度更大的 tool ，其中包含许多的小 tool 。
2. [compress agent](docs/todo_compress_agent.md)：一个通过压缩上下文实现承载超长上下文的Agent。
3. [follow up question](docs/todo_follow_up_question.md):下一步推荐。
