ChangeMessageVisibility が機能することを確認する試験

試験内容

* VisibilityTimeout はデフォルトの30秒であると仮定
* メッセージを受信後27秒経過してから10秒おきにChangeMessageVisibilityを投げる
* Timeoutは30→40→50と10秒ずつ延伸
* ChangeMessageVisibility を送るタイミングは 27, 37, 47, .. 秒後
* 6回伸ばしたらメッセージを消して終了
* その間、同じメッセージが降ってこないことを簡易的に待ってる

試験結果: 成功

* ChangeMessageVisibility を伸ばすあいだメッセージは抑制された
