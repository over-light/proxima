## CLI wallet program `proxi`

`proxi` is a small CLI program with basic tools for Proxima. Please, do not expect perfect UX :) 

The program can be compiled by typing `go install` in the `<root>/proxi` directory of the Proxima source.

Commands `proxi -h`, `proxi db -h`, `proxi node -h`, etc. will display help text. 

Most of the commands require a wallet configuration profile in the working directory.
Some commands, for example `proxi util ed25519` are completely stand alone and may not require any config profile.

`proxi` configuration profile usually is file `proxi.yaml`. If we use a file with another name, 
say `proxi2.yaml`, we have to specify it explicitly in the command line with flag `-c` 
and profile file name without extension, for example `proxi node balance -c proxi2`.

`proxi` commands has a form `proxi <cmd group> <subcommand> <args and flags>`, where `<cmd group>` is one of the following:

* `proxi util` all kind of helper subcommands
* `proxi init` is for admin subcommands, for initialization of the database. config profiles and similar 
* `proxi db`  for admin subcommands which access multi-state and txStore database directly. They will all fail if the node is running. 
Note that direct access to the Badger database may change permissions of some files and the node can fail to open it 
* if run on different user
* `proxi snapshot` for subcommands related to snapshots
* `proxi node` many subcommands which accesses node via API. They all require a configuration profile and an endpoint in the running node

### 1. Create a configuration profile and the wallet

The command `proxi init wallet` asks for entropy and generates private key from the provided seed and system randomness.
It also creates configuration profile `proxi.yaml`.

The file will contain something like this (with comments):

```yaml
private_key: af274b0363f484f8d113a9e17831ff3acd285fd152c5179db42ab0ff976e23153a51eabb1c19f1b5e784d086a6bf176c8ada3c248f25da93f7362c35eb1fc660
account: a(0x7450c426206c4164bc84ff30a14bdf72603b563e26a1d43973bc67cdb59033d8)
sequencer_id: <own sequencer ID>
api:
  endpoint: http://127.0.0.1:8000
tag_along:
  sequencer_id: 6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc
  fee: 500
spammer:
  bundle_size: 5
  output_amount: 1000
  pace: 25
  tag_along:
    fee: 50
    sequencer_id: <sequencer ID hex encoded>
  target: <target address in EasyFL format>
```

**Usually adjustments are needed to complete the profile**. 

`wallet.private_key` contains hex encoded raw data of the ED25519 private key. The file must be kept secret 
because of this private key. 

`wallet.account` contains address constraint in the _EasyFL_ format which matches the private key. It usually has the form of `a(0x...)`, which is the
_EasyFL_ script of the ED25519 lock.

`sequencer_id` is an optional field. It is irrelevant if you do not run sequencer. It contains `sequencer ID` of the sequencer controlled by this wallet. 
It is necessary so that to access sequencer controlled by this private key with the `proxi node seq withdraw ..` command. 

`api.endpoint` must contain URL for the node's API in the form of `http://<ip>:<port>`. **It must be set to the address of some public access point**

`tag_along.sequencer_id` is a mandatory field for any commands which create transactions, such as `proxi node transfer`.
It must contain a sequencer ID which is used as a tag-along sequencer. 
Each issued transaction will contain so-called _tag-along output_.
The *tag-along output* simply sends the amount of tokens specified in `tag_along.fee` to the sequencer in `tag_along.sequencer_id`. 
The sequencer will consume the *tag-along output* in its transaction. This will pull the transaction into the next ledger state. 
By default, `proxi.yaml` is initialized with the static constant of 
bootstrap sequencer ID `6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc`. 

### How to understand transaction and other IDs
Transaction ID in Proxima is a 32-byte array. First 5 bytes are the timestamp of the transaction, the byte at index 6 contains number of outputs produced 
by the transaction minus 1, the rest 26 bytes are taken from `blake2b` hash of the raw transaction bytes.

The transaction ID or its short (trimmed) form is often displayed like this:

`[58514|30sq]029d612d07f0d235c627b720f70fe5c84ac6b0f7f296097197fed5`
or
`[58565|0br]018b9b..`

Here `58514` is slot number. The `sq` means it is a non-branch sequencer transaction and `30` is number of ticks in the slot.
The rest is hex-encoded 26 bytes of transaction hash, prefixed with the one byte with the number of produced outputs minus 1.

If `br` is used instead of `sq`, it means it is a branch transaction. Branch transactions always have `0` ticks, i.e. they are
_on the slot boundary_.
If `sq` and `br` are skipped, it is an ordinary, non-sequencer transaction, produced by user wallet. 

The output (aka UTXO) on the ledger belongs to a transaction which produced it and index in the transaction from 0 to 255. 
Index is displayed as a postfix of the transaction ID.

For example `[58579|25]010b55301a97884f3fd8b4f44ef2c682b81ecda38d2b35a2116bfb[3]` is output of the non-sequencer transaction with index 3. 
Short for of the same output (only for display) would be `[58579|25]010b55..[3]`

Usual ED25519 address takes form `a(0x370563b1f08fcc06fa250c59034acfd4ab5a29b60640f751d644e9c3b84004d0)`, which contains 
hash of the public key. We will skip details here.

Chain ID is a 32-byte array. It is displayed in the hex-encoded form, often with prefix `$/`. 

For example `$/6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc` is the pre-defined constant chain ID of the bootstrap 
(genesis) chain.

### Some useful `proxi node` commands

* `proxi node lrb` displays **latest reliable branch (LRB)** info. _LRB_ represents the ledger state which is contained
by all the current healthy ledger states, i.e., it is the **consensus ledger state** with high probability.
   Usually we require our transaction to be included in the LRB or earlier branches. 

   If LRB is more than a few slots back from now, it may indicate that the node is not synced with the network. 

   LRB information also contains ID of the sequencer which produced the branch, total supply of tokens on the ledger state and the _ledger coverage_ of the branch.
   Initial token balance of the ledger at genesis in testnet is 1.000.000.000.000.000.000 tokens
   The total supply on the LRB ledger state is constantly changing, according to the inflation rules.

   The _ledger coverage_ must be > of the supply for the branch to be _healthy_. The maximum possible value of the ledger coverage is _2 x supply_.

* `proxi node balance` displays token balance on the usual (ED25519) address and on chains, controlled by the wallet's account in the LRB branch.  
   Token balance is the sum of tokens contained in non-chain outputs plus the sum of balances contained in chain outputs. 
The command also displays _delegations_ to sequencers.

* `proxi node mychains` displays all chain outputs controlled by the account on the LRB state

* `proxi node transfer <amount> -t "<target address>"` sends tokens from the wallet's account to the target address.
  For example, command `proxi node transfer 1000 -t "a(0x370563b1f08fcc06fa250c59034acfd4ab5a29b60640f751d644e9c3b84004d0)"`
  sends 1000 tokens to the specified address. The transfer transaction will contain so-called **tag-along** output with **tag-along fee**
  paid to the **tag-along sequencer** configured in the `proxi.yaml`.
  Flag  `-v` (or `--verbose`) will make command to display the whole transfer transaction. It is a good chance to get acquainted with the Proxima's UTXO transaction model.

* `proxi node compact` transfers tokens to itself by compacting outputs in the account. It is useful when account contains too much outputs. 
   This often happens as a result of the spamming. 
   Note that than `compact` command still requires tag-along fee. 

* `proxi node utxo` displays outputs (UTXOs) in the account

* `proxi node info` displays info of the node

* `proxi node getfunds` requests funds from a faucet server  
  The following settings can be used to specify a faucet server (addr:port) in `proxi.yaml`:

  ```yaml
  faucet:
      port:  9500
      addr:  113.30.191.219
  ```

### 2. Run spammer from the wallet

Spammer is used as a testing tool and to study the behavior of the system. 
Spammer periodically sends tokens from the wallet's account to the target address in bundles of transactions. 
It waits until each bundle of transactions reaches finality before sending the next one. 
The optional flag `-e` sets depth below LRB (_latest reliable branch_) transaction must be waited to reach. 
Default is `2`

Spammer is run with the command `proxi node spam -e 1`.

The bundle of transactions is a chain of transactions, which consumes output of the previous. 
Only the last one (tip of the batch) contains tag-along output. 

Configured tag-along sequencer consumes the output (the tip of the batch). 
This way it pulls the whole bundle of transactions into the ledger state with one tag-along fee amount.

As per current ledger constraints, one spammer can achieve maximum 1 TPS of the transfer transactions. 
In Proxima the rate is limited per address (per user). It is 1 TPS for non-sequencers (assuming no conflicting transactions are issued).
Higher total TPS can be reached only by multiple users. 
