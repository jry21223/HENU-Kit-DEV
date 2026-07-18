---
status: accepted
---

# HENUKit Console 集成采用 OpenAPI-first

Console Gateway、Platform Core 与各 Active Product Module 先冻结版本化 OpenAPI 3.1 契约，再实现服务和前端；TypeScript 客户端与必要的 Go 类型从契约生成，CI 同时检查契约规则、生成产物和实现一致性。每个数据 Owner 维护自己的业务契约，Console Gateway 只组合这些契约，不以已经写出的实现反向定义产品边界。
