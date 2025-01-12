## Delegation

### Introduction

_Delegation_ is a process of making your tokens available to another token holder, normally to _sequencer_. 
The sequencer will be able to generate inflation from the delegated tokens, however, it will not be able to steal 
delegated tokens. 

One way of generating inflation from token holdings is running a sequencer. This normally includes running the node.
_Delegation_ is an alternative to sequencing: one can generate inflation from their tokens without running sequencer and node, 
without any operations cost, because all the trouble is outsourced to somebody else.

### How to delegate?

At the core of _delegation_ is chain mechanism: inflation is generated, i.e. tokens are created "out of thin air" by building chains.
User creates a new chain output with all the tokens he wants to delegate on it. This can be achieved with following command:

`proxi node delegate <amount of tokens> -q <target sequencer ID>`

The `<target sequencer ID>` is hex-encoded chain ID of the target sequencer of your liking. 
Normally target sequencer for delegation is selected based on its market parameters such as uptime and effective inflation rate. 
In the testnet you can find all active sequencers with the following command:

`proxi node allchains`

Choose a chain which is run by some sequencer as a delegation target, preferably the one active in the last several slots from now.

For example, in the following command, the chain `6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc` is the ony sequencer
available:
```text
Command line: 'proxi node allchains'
using profile: ./proxi.yaml
using API endpoint: http://127.0.0.1:8000, default timeout
successfully connected to the node at http://127.0.0.1:8000
Latest reliable branch (LRB) ID: [10667|0br]0184e71defcf9be900af01c3347f4e38d89856194009587b8daafd, 0 slot(s) from now. Now is 10667|194

list of all chains (2)

 0: $/6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc, sequencer: boot.b0 (2083/856)
      balance         : 999_230_164_470_673
      controller lock : a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98)
      output          : [10667|0br]0184e71defcf..[0]

 1: $/6453068190f2088f8620c3f906eb756c23200a02f8ad42c1f1fb5ffa67f1bc27, sequencer: NO
      balance         : 200_001_116_250
      controller lock : delegationLock(2, a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98), a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d), 0x000028ed62, u64/200000000000)
      output          : [10665|40]019a2efce9dd..[0]
```

The command will create new chain with new delegation ID (which is chain ID). For example, the following  command created delegation outputs
with _delegation ID_ `2fed1ce675e654c973b314459447ff8b895e826ed6356400cd20d25b32e508cf`.

```text
Command line: 'proxi node delegate 150000000000 -q 6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc'
using profile: ./proxi.yaml
using API endpoint: http://127.0.0.1:8000, default timeout
successfully connected to the node at http://127.0.0.1:8000
wallet account is: a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d)
delegation target will be controller a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98) of the sequencer $/6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc
using tag_along sequencer: 6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc
Latest reliable branch (LRB) ID: [10726|0br]01ef03ef1bf8b5dd2d3d0c1088cead27bc466e393e76144aac8135, 0 slot(s) from now. Now is 10726|239
delegate amount 150_000_000_000 to controller a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98) (plus tag-along fee 50)? (Y/n)

delegation ID: $/2fed1ce675e654c973b314459447ff8b895e826ed6356400cd20d25b32e508cf

Tracking inclusion of [10726|239]02ca7b0d08cd1e0d6f6c92da00ee10221adfc209dd93ec587e573a (hex=000029e6ef02ca7b0d08cd1e0d6f6c92da00ee10221adfc209dd93ec587e573a):
  finality criterion: strong, slot span: 2, strong inclusion threshold: 2/3
   weak score: 0%, strong score: 0%, slot span 10726 - 10727 (2), included in LRB: false, LRB is slots back: 0
   weak score: 0%, strong score: 0%, slot span 10726 - 10727 (2), included in LRB: false, LRB is slots back: 0
   weak score: 0%, strong score: 0%, slot span 10726 - 10727 (2), included in LRB: false, LRB is slots back: 0
   ...
   weak score: 50%, strong score: 50%, slot span 10727 - 10728 (2), included in LRB: true, LRB is slots back: 0
   weak score: 50%, strong score: 50%, slot span 10727 - 10728 (2), included in LRB: true, LRB is slots back: 0
   weak score: 100%, strong score: 100%, slot span 10728 - 10729 (2), included in LRB: true, LRB is slots back: 0
```
### How to check my delegations?
Commands `proxi node mychains -s` or `proxi node mychains -s -v` lists all chains, which are delegations controlled 
from the current wallet. For example:

```text
Command line: 'proxi node mychains -d -v'
using profile: ./proxi.yaml
using API endpoint: http://127.0.0.1:8000, default timeout
successfully connected to the node at http://127.0.0.1:8000
Latest reliable branch (LRB) ID: [10750|0br]01050538356eeee0a17a46266cb1ef9a09d4a3ef02c1cb3e42b1e0, 0 slot(s) from now. Now is 10750|149

List of delegations in account a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d)

$/2fed1ce675e654c973b314459447ff8b895e826ed6356400cd20d25b32e508cf   150_000_097_977            -> a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98)
        +97_977 since 10726|239 (24 slots), 4_082 per slot, start amount 150_000_000_000, annual rate: ~8.38%

$/6453068190f2088f8620c3f906eb756c23200a02f8ad42c1f1fb5ffa67f1bc27   200_001_615_000            -> a(0x43ceee694015e327a85c66c9c1a0c0bb8c7de37f19d5e8a9ec86d1eb81931d98)
        +1_615_000 since 10477|98 (273 slots), 5_915 per slot, start amount 200_000_000_000, annual rate: ~9.11%

Total delegated in 2 outputs: 350_001_712_977
```

By repeating commands `proxi node mychains -s` or `proxi node mychains -s -v` you will see how newly created tokens are 
coming to the balance of your delegated chain roughly each 20 sec.

### How to reclaim delegated tokens back?
It works by simply destroying the delegation chains with the command:

`proxi node delchain <delegation ID hex>`.

For example:

```text
Command line: 'proxi node delchain 2fed1ce675e654c973b314459447ff8b895e826ed6356400cd20d25b32e508cf'
using profile: ./proxi.yaml
using API endpoint: http://127.0.0.1:8000, default timeout
successfully connected to the node at http://127.0.0.1:8000
wallet account will be used as target: a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d)
using tag_along sequencer: 6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc
Deleting chain:
   chain id: $/2fed1ce675e654c973b314459447ff8b895e826ed6356400cd20d25b32e508cf
   tag-along fee 50 to the sequencer $/6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc
   source account: a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d)
   chain controller: a(0x77c01a4e09a49a85c77db28d7891108d4073bbd0f534c27484be7366bc15610d)
proceed?: (Y/n)
Tracking inclusion of [10797|99]010a5331cbf6ebd6967742f22280e6302d61cb4d01ce64595b79b2 (hex=00002a2d63010a5331cbf6ebd6967742f22280e6302d61cb4d01ce64595b79b2):
  finality criterion: strong, slot span: 2, strong inclusion threshold: 2/3
   weak score: 0%, strong score: 0%, slot span 10796 - 10797 (2), included in LRB: false, LRB is slots back: 0
   weak score: 0%, strong score: 0%, slot span 10796 - 10797 (2), included in LRB: false, LRB is slots back: 0
   weak score: 0%, strong score: 0%, slot span 10796 - 10797 (2), included in LRB: false, LRB is slots back: 0
  ...
   weak score: 50%, strong score: 50%, slot span 10797 - 10798 (2), included in LRB: true, LRB is slots back: 0
   weak score: 100%, strong score: 100%, slot span 10798 - 10799 (2), included in LRB: true, LRB is slots back: 0
```

All tokens from the chain are returned to the normal wallet's `ED25519` address.

**Important!**

If the command `proxi node delchain <delegation ID hex>` does not confirm the transaction in 30-40 sec, it means it was orphaned. 
It can happen because there are significant chances of race condition between sequencer and the deletion transaction, 
so it looks like "the sequencer does not want to give my tokens back". 

This, however, is not a risk to lose your tokens because:
- normally, after repeating the command `proxi node delchain <delegation ID hex>` one or several time everything works out. 
Probability of the race condition is ~50%, so the coin falling one side forever is impossible.
- even if sequencer which is delegation target becomes malicious and ignores your transaction, it is easy to switch to alternative one
or even several of them. 

Testnet implementation of the _delegation lock_ is very simplistic and experimental. There are a lot of other possibilities to 
avoid race conditions with _EasyFL_ scripts.

### How it works?

The new delegation chain output is locked with so-called **delegation lock**, a special *EasyFL* script. It can be seen as a _smart contract_. 
The *delegation lock* script allows two different private keys (addresses) to unlock the chain, but each under different conditions:

- the _original owner_ can unlock and consume the output any time without limitations
- the _delegation target_ (sequencer) can unlock and consume output only under following conditions:
  - if consuming transaction is on an even slot (i.e. every second slot)
  - if chain successor contains at least the same amount of tokens as predecessor (no possibility of stealing)

Race condition can occur when owner consumes transaction in the same slot as delegation target. 
Wallet software can prevent it even if delegation target is malicious.  

### What is minimum amount of delegation?

It is limited by the amount of inflation which can be generated in 1 slot. In the first slot since genesis `33.000.000` 
will generate 1 token of inflation per slot. Less than that will generate 0 inflation due to rounding. 
Due to adjustment of the inflation rate over time, this minimum amount will grow. 

Normally, meaningful amount of delegation is from `700.000.000` tokens and upwards. 

### How much inflation is generated by delegation?

Maximal inflation generated by the non-sequencer chain is defined by the ledger rules. In the testnet it is tail inflation
corresponding to ~10% annual inflation first year, later gradually diminishing.

In theory, delegation target can take all the generated inflation for itself and leave 0 for the owner (it cannot steal delegated tokens though).
In that case, owner can reclaim its delegation back and delegate it to another, less greedy sequencer. 

The point is, the fraction of the generated inflation delegation target takes from newly created amounts to itself is up to the market. 
In the extreme (though realistic) case %100 of inflation will be returned to the owner. 
Why? Because sequencer has inherent interest to attract more capital to its ledger
coverage, so sequencers will compete by price to more attractive to token holders who want to delegate their holdings.

In the testnet, sequencer "shaves" 1/10th of the generated inflation and takes for itself (it is configurable on sequencer).
So, maximum possible 10% annual percent of inflation will become ~9% for the delegating token holder. 
That is more or less fair, because sequencers incur costs and wants to cover it, while other token holders has 0 cost of delegating tokens.

### Why delegate?
Each token holder are incentivized to earn from inflation (subject to natural rules of minimum amounts). 
It means, if token holder is lazy and its holding stay in a passive address, he will suffer the opportunity cost of inflation (dilution).

This is by design: we want every token will be in the ledger coverage, therefore delegation will contribute to the security of the ledger
and will be rewarded by inflation tokens.

