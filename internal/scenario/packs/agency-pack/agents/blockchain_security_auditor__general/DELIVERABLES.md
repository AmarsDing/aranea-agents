## 📋 你的技术交付物
### 重入漏洞分析
```solidity
// 有漏洞：经典重入——状态在外部调用后更新
contract VulnerableVault {
    mapping(address => uint256) public balances;

    function withdraw() external {
        uint256 amount = balances[msg.sender];
        require(amount > 0, "No balance");

        // BUG：状态更新之前的外部调用
        (bool success,) = msg.sender.call{value: amount}("");
        require(success, "Transfer failed");

        // 攻击者在此行执行前重入 withdraw()
        balances[msg.sender] = 0;
    }
}

// 利用：攻击者合约
contract ReentrancyExploit {
    VulnerableVault immutable vault;

    constructor(address vault_) { vault = VulnerableVault(vault_); }

    function attack() external payable {
        vault.deposit{value: msg.value}();
        vault.withdraw();
    }

    receive() external payable {
        // 重入 withdraw——余额尚未清零
        if (address(vault).balance >= vault.balances(address(this))) {
            vault.withdraw();
        }
    }
}

// 已修复：检查-效果-交互 + 重入防护
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

contract SecureVault is ReentrancyGuard {
    mapping(address => uint256) public balances;

    function withdraw() external nonReentrant {
        uint256 amount = balances[msg.sender];
        require(amount > 0, "No balance");

        // 效果在交互之前
        balances[msg.sender] = 0;

        // 交互最后
        (bool success,) = msg.sender.call{value: amount}("");
        require(success, "Transfer failed");
    }
}
```

### 预言机操纵检测
```solidity
// 有漏洞：现货价格预言机——可通过闪电贷操纵
contract VulnerableLending {
    IUniswapV2Pair immutable pair;

    function getCollateralValue(uint256 amount) public view returns (uint256) {
        // BUG：使用现货储备——攻击者通过闪电交换操纵
        (uint112 reserve0, uint112 reserve1,) = pair.getReserves();
        uint256 price = (uint256(reserve1) * 1e18) / reserve0;
        return (amount * price) / 1e18;
    }

    function borrow(uint256 collateralAmount, uint256 borrowAmount) external {
        // 攻击者：1) 闪电交换扭曲储备
        //           2) 以膨胀的抵押品价值借款
        //           3) 偿还闪电交换——获利
        uint256 collateralValue = getCollateralValue(collateralAmount);
        require(collateralValue >= borrowAmount * 15 / 10, "Undercollateralized");
        // ... 执行借款
    }
}

// 已修复：使用时间加权平均价格 (TWAP) 或 Chainlink 预言机
import {AggregatorV3Interface} from "@chainlink/contracts/src/v0.8/interfaces/AggregatorV3Interface.sol";

contract SecureLending {
    AggregatorV3Interface immutable priceFeed;
    uint256 constant MAX_ORACLE_STALENESS = 1 hours;

    function getCollateralValue(uint256 amount) public view returns (uint256) {
        (
            uint80 roundId,
            int256 price,
            ,
            uint256 updatedAt,
            uint80 answeredInRound
        ) = priceFeed.latestRoundData();

        // 验证预言机响应——永不盲目信任
        require(price > 0, "Invalid price");
        require(updatedAt > block.timestamp - MAX_ORACLE_STALENESS, "Stale price");
        require(answeredInRound >= roundId, "Incomplete round");

        return (amount * uint256(price)) / priceFeed.decimals();
    }
}
```

### 访问控制审计清单
```markdown
# 访问控制审计清单
