# Agent API Contract

Round one uses `FinalReviewApiClient` mock responses with the same shapes planned for final-review-platform Agent API.

## Search Catalog

`GET /api/agent/catalog/search`

Parameters:

```json
{
  "query": "离散数学",
  "school": "河南大学",
  "college": "软件学院",
  "major": "网络工程",
  "grade": "2023级"
}
```

Returns published packages only.

## Get Package Detail

`GET /api/agent/packages/:id`

Returns package materials, price in integer fen, applicable school/college/major/grade, and access rules.

## Create Order

`POST /api/agent/orders`

The plugin sends:

```json
{
  "packageId": "pkg_discrete_math_2023",
  "channel": "langbot",
  "chatPlatform": "qq",
  "chatUserId": "123456",
  "chatGroupId": "987654",
  "buyerRemark": "用户通过 LangBot 购买课程资料"
}
```

The plugin never sends amount, price, paid, entitlement, or downloadUrl. The backend reads price from the package.

## Query Order

`GET /api/agent/orders/:id/status`

Returns local order state and whether delivery was created.

## Create Delivery

`POST /api/agent/deliveries/create`

Only paid orders may create delivery. The delivery link must be short-lived and must still require backend entitlement/download checks.

## Resend Delivery

`POST /api/agent/deliveries/resend`

Returns an existing valid delivery link; it must not create unlimited delivery records.

## Conversation Log

`POST /api/agent/conversations/log`

Reserved for the real integration round.
