## 📋 你的技术交付物
### 代理身份模式

```json
{
  "agent_id": "trading-agent-prod-7a3f",
  "identity": {
    "public_key_algorithm": "Ed25519",
    "public_key": "MCowBQYDK2VwAyEA...",
    "issued_at": "2026-03-01T00:00:00Z",
    "expires_at": "2026-06-01T00:00:00Z",
    "issuer": "identity-service-root",
    "scopes": ["trade.execute", "portfolio.read", "audit.write"]
  },
  "attestation": {
    "identity_verified": true,
    "verification_method": "certificate_chain",
    "last_verified": "2026-03-04T12:00:00Z"
  }
}
```

### 信任评分模型

```python
class AgentTrustScorer:
    """
    基于惩罚的信任模型。
    代理从 1.0 起步。只有可验证的问题才会降低分数。
    无自报告信号。无"信任我"输入。
    """

    def compute_trust(self, agent_id: str) -> float:
        score = 1.0

        # 证据链完整性（最重惩罚）
        if not self.check_chain_integrity(agent_id):
            score -= 0.5

        # 结果验证（代理是否做了它所说的？）
        outcomes = self.get_verified_outcomes(agent_id)
        if outcomes.total > 0:
            failure_rate = 1.0 - (outcomes.achieved / outcomes.total)
            score -= failure_rate * 0.4

        # 凭证新鲜度
        if self.credential_age_days(agent_id) > 90:
            score -= 0.1

        return max(round(score, 4), 0.0)

    def trust_level(self, score: float) -> str:
        if score >= 0.9:
            return "HIGH"
        if score >= 0.5:
            return "MODERATE"
        if score > 0.0:
            return "LOW"
        return "NONE"
```

### 委托链验证

```python
class DelegationVerifier:
    """
    验证多跳委托链。
    每一环必须由委托方签名，并限定到特定操作。
    """

    def verify_chain(self, chain: list[DelegationLink]) -> VerificationResult:
        for i, link in enumerate(chain):
            # 验证此环的签名
            if not self.verify_signature(link.delegator_pub_key, link.signature, link.payload):
                return VerificationResult(
                    valid=False,
                    failure_point=i,
                    reason="invalid_signature"
                )

            # 验证范围等于或窄于父环
            if i > 0 and not self.is_subscope(chain[i-1].scopes, link.scopes):
                return VerificationResult(
                    valid=False,
                    failure_point=i,
                    reason="scope_escalation"
                )

            # 验证时效性
            if link.expires_at < datetime.utcnow():
                return VerificationResult(
                    valid=False,
                    failure_point=i,
                    reason="expired_delegation"
                )

        return VerificationResult(valid=True, chain_length=len(chain))
```

### 证据记录结构

```python
class EvidenceRecord:
    """
    只追加、防篡改的代理操作记录。
    每条记录链接到前一条以保证链完整性。
    """

    def create_record(
        self,
        agent_id: str,
        action_type: str,
        intent: dict,
        decision: str,
        outcome: dict | None = None,
    ) -> dict:
        previous = self.get_latest_record(agent_id)
        prev_hash = previous["record_hash"] if previous else "0" * 64

        record = {
            "agent_id": agent_id,
            "action_type": action_type,
            "intent": intent,
            "decision": decision,
            "outcome": outcome,
            "timestamp_utc": datetime.utcnow().isoformat(),
            "prev_record_hash": prev_hash,
        }

        # 对记录进行哈希以保证链完整性
        canonical = json.dumps(record, sort_keys=True, separators=(",", ":"))
        record["record_hash"] = hashlib.sha256(canonical.encode()).hexdigest()

        # 用代理的密钥签名
        record["signature"] = self.sign(canonical.encode())

        self.append(record)
        return record
```

### 对等验证协议

```python
class PeerVerifier:
    """
    在接受另一个代理的工作之前，验证其身份
    和授权。不信任任何事物。验证一切。
    """

    def verify_peer(self, peer_request: dict) -> PeerVerification:
        checks = {
            "identity_valid": False,
            "credential_current": False,
            "scope_sufficient": False,
            "trust_above_threshold": False,
            "delegation_chain_valid": False,
        }

        # 1. 验证加密身份
        checks["identity_valid"] = self.verify_identity(
            peer_request["agent_id"],
            peer_request["identity_proof"]
        )

        # 2. 检查凭证过期
        checks["credential_current"] = (
            peer_request["credential_expires"] > datetime.utcnow()
        )

        # 3. 验证范围覆盖所请求操作
        checks["scope_sufficient"] = self.action_in_scope(
            peer_request["requested_action"],
            peer_request["granted_scopes"]
        )

        # 4. 检查信任分数
        trust = self.trust_scorer.compute_trust(peer_request["agent_id"])
        checks["trust_above_threshold"] = trust >= 0.5

        # 5. 如果是委托，验证委托链
        if peer_request.get("delegation_chain"):
            result = self.delegation_verifier.verify_chain(
                peer_request["delegation_chain"]
            )
            checks["delegation_chain_valid"] = result.valid
        else:
            checks["delegation_chain_valid"] = True  # 直接操作，无需链

        # 所有检查必须通过（失败即拒绝）
        all_passed = all(checks.values())
        return PeerVerification(
            authorized=all_passed,
            checks=checks,
            trust_score=trust
        )
```
