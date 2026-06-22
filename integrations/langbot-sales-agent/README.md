# final-review-sales-agent

LangBot SDK plugin for the final-review-platform sales front desk. It lets users query course packages, create mock WeChat Native payment orders, and receive delivery links after backend-confirmed payment.

Current status: first-round mock skeleton. Real WeChat Pay notify, paid entitlement, PDF delivery, and live final-review-platform Agent API are not implemented in this plugin round.

## What This Plugin Does

- Provides LangBot Command, Tool, and Event Listener components.
- Supports `!资料`, `!购买`, `!订单`, `!重发`, `!帮助` through the sales guard listener.
- Supports `!final_review 资料/购买/订单/重发/帮助` through the official Command component.
- Provides six tools for a controlled local agent: `search_catalog`, `get_package_detail`, `create_order`, `query_order`, `create_delivery`, `resend_delivery`.
- Generates temporary QR code images from mock WeChat Native `codeUrl`.
- Blocks unsafe sales language such as 包过、必中、内部资料、泄题.

## Security Boundary

The plugin never:

- handles WeChat Pay notify;
- marks orders as paid;
- grants entitlement;
- sends paid PDF files;
- accepts or sends price/amount override fields;
- stores merchant private keys, API v3 keys, certificates, or course PDFs.

Payment and delivery must be confirmed by final-review-platform backend APIs.

## Setup

```bash
pip install -r requirements.txt
```

Copy `.env.example` to `.env` only for local development. Do not commit `.env`.

## Debug

```bash
lbp run
```

`lbp run` requires a LangBot Plugin Runtime connection. If runtime is unavailable, unit tests still validate the mock sales flow.

## Test

```bash
python -m pytest tests
```

## Mock Commands

```text
!帮助
!资料 离散数学
!购买 离散数学
!订单
!重发 ord_xxx
```

Round one uses in-memory mock orders. Restarting the plugin keeps conversation state in `.tmp/sales_state.json`, but mock order records are not a production database.
