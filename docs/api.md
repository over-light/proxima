# TX API

* [compile_script](#compile_script)
* [decompile_bytecode](#decompile_bytecode)
* [parse_output_data](#parse_output_data)
* [parse_output](#parse_output)
* [get_txbytes](#get_txbytes)
* [get_parsed_transaction](#get_parsed_transaction)
* [get_vertex_dep](#get_vertex_dep)


## compile_script
Compiles EasyFL script in the context of the ledger of the node and returns bytecode

`/txapi/v1/compile_script?source=<script source in EasyFL>`

Example:
``` bash
curl -L -X GET http://localhost:8000/txapi/v1/compile_script?source="slice(0x0102,0,0)"
```

```json
{
  "bytecode": "1182010281008100"
}
```

## decompile_bytecode
Decompiles bytecode to EasyFL script

`/txapi/v1/decompile_bytecode?bytecode=<hex-encoded bytecode>`

Example:
``` bash
curl -L -X GET http://localhost:8000/txapi/v1/decompile_bytecode?bytecode=1182010281008100
```

```json
{
  "source": "slice(0x0102,0,0)"
}
```

## parse_output_data
By given raw data of the output, parses it as lazyarray
and decompiles each of constraint scripts. Returns list of decompiled constraint scripts
Essential difference with the parse-output is that it does not need to assume particular LRB

`/txapi/v1/parse_output_data?output_data=<hex-encoded output binary>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/txapi/v1/parse_output_data?output_data=40060945b186b512ee27fa402345b9a05950f002387d659a3d1723235fabbf8b32decadc6807c24f85bffc2fff21c9482645c6a3d2a2de2a8caec2bc46c0c95c668ba36107330d6c63b9353b1a186fcd9e38f5320002000b49d8810286b512ee27fa401d5042876c6f63312e65318400004adb8400000c2f8800000000000000000849db8364355f8102'
```

```json
{
  "data": "40060945b186b512ee27fa402345b9a05950f002387d659a3d1723235fabbf8b32decadc6807c24f85bffc2fff21c9482645c6a3d2a2de2a8caec2bc46c0c95c668ba36107330d6c63b9353b1a186fcd9e38f5320002000b49d8810286b512ee27fa401d5042876c6f63312e65318400004adb8400000c2f8800000000000000000849db8364355f8102",
  "constraints": [
    "amount(z64/199092909636160)",
    "a(0x5950f002387d659a3d1723235fabbf8b32decadc6807c24f85bffc2fff21c948)",
    "chain(0xd2a2de2a8caec2bc46c0c95c668ba36107330d6c63b9353b1a186fcd9e38f532000200)",
    "sequencer(2, z64/199092909636160)",
    "or(0x6c6f63312e6531,0x00004adb,0x00000c2f,0x0000000000000000)",
    "inflation(z64/6567263, 2)"
  ],
  "amount": 199092909636160,
  "lock_name": "",
  "chain_id": "d2a2de2a8caec2bc46c0c95c668ba36107330d6c63b9353b1a186fcd9e38f532"
}
```

## parse_output
By given output ID, finds raw output data on LRB state, parses the it as lazyarray
and decompiles each of constraint scripts. Returns list of decompiled constraint scripts

`/txapi/v1/parse_output?output_id=<hex-encoded output ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/txapi/v1/parse_output?output_id=80003e0a00015dc6362a7e6ed5a422b2edc5e46d262d690cfdea81dd997243fd00'
```

```json
{
  "data": "40060b45ab8800038d7ff693a50e2345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c2a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d181028800038d7ff693a50e1d504287626f6f742e623084000006ad840000035c8800000000000000006151d7880000000000386580d1022cee903827166af9c0257be156222ae34c9279831a294fb5213647a3bcbe7a3e203d100f3ec2095fb076c65ed1d29a680c05c7993d43e7fdd0a779e8f783943a50b0a1ba3b373830cc2f447a030edb35810281ff",
  "constraints": [
    "amount(u64/1000005667366158)",
    "a(0x033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d)",
    "chain(0x6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc000200)",
    "sequencer(2, u64/1000005667366158)",
    "or(0x626f6f742e6230,0x000006ad,0x0000035c,0x0000000000000000)",
    "inflation(0x0000000000386580, 0x022cee903827166af9c0257be156222ae34c9279831a294fb5213647a3bcbe7a3e203d100f3ec2095fb076c65ed1d29a680c05c7993d43e7fdd0a779e8f783943a50b0a1ba3b373830cc2f447a030edb35, 2, 0xff)"
  ],
  "amount": 1000005667366158,
  "chain_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc"
}
```

## get_txbytes
By given transaction ID, returns raw transaction bytes (canonical form of tx) and metadata (if it exists)

`/txapi/v1/get_txbytes?txid=<hex-encoded transaction ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/txapi/v1/get_txbytes?txid=000032020101b7f9dced0a8cf56fe4c7ea248ddd3c38f59b495bf93b71e8a46a
```

```json
{
  "tx_bytes": "400b46400221000032014b001325a36f81e60c5d24e9f3ec917d85e694f2b372ffae8f9ae06f002100003201010101d46fc3018b424bc4c0d5c1386cc20966f81b9e1f1ea3a72813010f40020940030001ff03000200020000fa40028a40060945b186b512b5f3881e2345b9a05950f002387d659a3d1723235fabbf8b32decadc6807c24f85bffc2fff21c9482645c6a3d2a2de2a8caec2bc46c0c95c668ba36107330d6c63b9353b1a186fcd9e38f5320002000b49d8810286b512b5f3881e1d5042876c6f63312e62308400004a178400000c128800000000000000000849db83387ec681026c40020345b1806549cea100003201010101d46fc3018b424bc4c0d5c1386cc20966f81b9e1f1ea3a7281301c05fb69739fb3a8562a40da80e37c4998fd96313141edd012d5f987eb38887d8e7d20ae6a6732752a7b0c553872b6477af4ce3aa576cb874b4f424ea72bb25a30560968751c3bc6278ea65ef8cd8bd808fdd6b9e672d12e706b2b845e0671524ec68036668d4d828b6c3beb2c970deee3aa17409051588ec9fb6bbf2c62a57eb3f0a8b110b987bf78c78ad49e4ceca66ca8c253283995c213436d8b40aa11604b38202000105000032020006b512b5f3881e20049d77549f8aede2eaab32ae65b6e317c6400d7760379aebcc0232437f176f1f02000000020000",
  "tx_metadata": {
    "ledger_coverage": 1989918454922344,
    "slot_inflation": 36703460,
    "supply": 1000452558169838
  }
}
```


## get_parsed_transaction
By the given transaction ID, returns parsed transaction in JSON form. The JSON form contains all elements
of the transaction except signature, but it is not a canonical form. Primary purpose of JSON form of the transaction
is to use it in frontends, like explorers and visualizers.

`/txapi/v1/get_parsed_transaction?txid=<hex-encoded transaction ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/txapi/v1/get_parsed_transaction?txid=8000001400017a49603b8371e0063c4c0fb8fb7d91dd9968f308907d9ff2f8b7'
```

```json
{
  "id": "8000001400017a49603b8371e0063c4c0fb8fb7d91dd9968f308907d9ff2f8b7",
  "total_amount": 1000000108414869,
  "total_inflation": 1959219,
  "is_branch": true,
  "sequencer_tx_data": {
    "sequencer_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc",
    "sequencer_output_index": 0,
    "stem_output_index": 1,
    "milestone_data": {
      "name": "boot.b0",
      "minimum_fee": 0,
      "chain_height": 34,
      "branch_height": 18
    }
  },
  "sender": "a(0x033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d)",
  "signature": "6cc2dab71a8128ec5feb992af3247027acc05c360d62fc794b638b901828f586cb0e561ed07fbff49334d8bbe64b5484c075b5d4013423c408db37b77cbece09c07e104fcbec1daf388ffe50a6fd3ddf006d1e24a384ff81277fee6eff808738",
  "inputs": [
    {
      "output_id": "80000013190028be2c49083ae99f41d5fd3950e35bea475f1344562e898adf9d00",
      "unlock_data": "40030001ff03000200"
    },
    {
      "output_id": "8000001300017eaa649b97b3adfbd6ab054aad50b8705ddd7eb7843cf17631c001",
      "unlock_data": "0000"
    }
  ],
  "outputs": [
    {
      "data": "40060b45ab8800038d7eab3cc7952345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c2a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d181028800038d7eab3cc7951d504287626f6f742e6230840000002284000000128800000000000000006151d7880000000000386580d102f8ad2db2c9748968c1612ee12cf91688e084527b221b03a3a1e5d3cc23f5ee5db5f2493c06d03f9161ad4afb18379877083de5c80a0b8b3e5a194f570c7a65502c04cc806621857e046b7d64b433c733810281ff",
      "constraints": [
        "amount(u64/1000000108414869)",
        "a(0x033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d)",
        "chain(0x6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc000200)",
        "sequencer(2, u64/1000000108414869)",
        "or(0x626f6f742e6230,0x00000022,0x00000012,0x0000000000000000)",
        "inflation(0x0000000000386580, 0x02f8ad2db2c9748968c1612ee12cf91688e084527b221b03a3a1e5d3cc23f5ee5db5f2493c06d03f9161ad4afb18379877083de5c80a0b8b3e5a194f570c7a65502c04cc806621857e046b7d64b433c733, 2, 0xff)"
      ],
      "amount": 1000000108414869,
      "chain_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc"
    },
    {
      "data": "40020b45ab8800000000000000002445c6a18000001300017eaa649b97b3adfbd6ab054aad50b8705ddd7eb7843cf17631c001",
      "constraints": [
        "amount(u64/0)",
        "stemLock(0x8000001300017eaa649b97b3adfbd6ab054aad50b8705ddd7eb7843cf17631c001)"
      ],
      "amount": 0
    }
  ],
  "tx_metadata": {
    "ledger_coverage": 1999987795299295,
    "slot_inflation": 6055219,
    "supply": 1000000109414869
  }
}
```

## get_vertex_dep
By the given transaction ID, returns compressed for of the DAG vertex. Its primary use is DAG visualizers

`/txapi/v1/get_vertex_dep?txid=<hex-encoded transaction ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/txapi/v1/get_vertex_dep?txid=8000001400017a49603b8371e0063c4c0fb8fb7d91dd9968f308907d9ff2f8b7'
```

```json
{
  "id": "8000001400017a49603b8371e0063c4c0fb8fb7d91dd9968f308907d9ff2f8b7",
  "total_amount": 1000000108414869,
  "total_inflation": 1959219,
  "is_sequencer_tx": true,
  "is_branch": true,
  "sequencer_input_index": 0,
  "stem_input_index": 1,
  "input_dependencies": [
    "80000013190028be2c49083ae99f41d5fd3950e35bea475f1344562e898adf9d00",
    "8000001300017eaa649b97b3adfbd6ab054aad50b8705ddd7eb7843cf17631c001"
  ]
}
```

# WebSocket API
* [dag_vertex_stream](#dag_vertex_stream)
## dag_vertex_stream
TODO

# General API
* [get_ledger_id](#get_ledger_id)
* [get_account_outputs](#get_account_outputs)
* [get_account_parsed_outputs](#get_account_parsed_outputs)
* [get_account_simple_siglocked](#get_account_simple_siglocked)
* [get_outputs_for_amount](#get_outputs_for_amount)
* [get_nonchain_balance](#get_nonchain_balance)
* [get_chain_outputs](#get_chain_outputs)
* [get_chain_output](#get_chain_output)
* [get_output](#get_output)
* [query_inclusion_score](#query_inclusion_score)
* [submit_tx](#submit_tx)
* [sync_info](#sync_info)
* [node_info](#node_info)
* [peers_info](#peers_info)
* [get_latest_reliable_branch](#get_latest_reliable_branch)
* [check_txid_in_lrb](#check_txid_in_lrb)
* [last_known_milestones](#last_known_milestones)
* [get_mainchain](#get_mainchain)
* [get_all_chains](#get_all_chains)
* [get_delegations_by_sequencer](#get_delegations_by_sequencer)


## get_ledger_id_data
GET ledger definitions in YAML format: `/api/v1/get_ledger_id`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_ledger_id_data'
```

```yaml
# Proxima ledger definitions
hash: 3541ce47bdfb5341bd36ef017a067ede8a59654fc495cddc85baaae2f8c8f4b7
functions:
  # BEGIN EMBEDDED function definitions
  #    function codes (opcodes) from 0 to 15 are reserved for predefined parameter access functions $i and $$i
  # BEGIN SHORT EMBEDDED function definitions
  #    function codes (opcodes) from 16 to 63 are reserved for 'SHORT EMBEDDED function codes'
  -
    sym: "fail"
    description: "fails with parameter as panic message, where '_' is replaced with space"
    funCode: 16
    numArgs: 1
    embedded: true
    short: true
  -
    sym: "slice"
    description: "slice($0,$1,$2) takes a slice of $0, from $1 to $2 inclusive. $1 and $2 must be 1-byte long"
    funCode: 17
    numArgs: 3
    embedded: true
    short: true
  -
    sym: "byte"
    description: "byte($0,$1) takes byte $1 of $0, returns 1-byte long slice. $1 must be 1-byte long"
    funCode: 18
    numArgs: 2
    embedded: true
    short: true
 

..... so on
```


## get_account_outputs
Get in general non-deterministic set of outputs because of random ordering and limits
`/api/v1/get_account_outputs?accountable=<EasyFL source form of the accountable lock constraint>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_account_outputs?accountable=a(0x033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d)'
```

```json
{
  "outputs": {
    "00001d486301153c8d8abe069e3c5e87fc34b721bb2e8886038138a014bd655200": "40020b45ab8800000000000f404c2345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d",
    "80001d4869017f347dce50e146eb8f27b7edd5b493897936d2940039cddd918801": "40020b45ab8800000000000f42402345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d",
    "80003cad00014db2b8d461642ccf92177fa83f3feb4a165d8a2ed8ac8281d2db00": "40060b45ab8800038d7f7c182c6b2345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c2a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d181028800038d7f7c182c6b1d504287626f6f742e6230840000043f84000002248800000000000000006151d7880000000000386580d103948987dc2a310986f2a9691b5b2dfcb6f634ce0d77bcb36fa4ffd0aeb1237886229954002d65b5cf4f9acee89b460896069413fdffe68994a1a36514cd01c4e8179ffa0c889079739e745425e0ed64bd810281ff"
  },
  "lrb_id": "80003cad00014db2b8d461642ccf92177fa83f3feb4a165d8a2ed8ac8281d2db"
}
```

## get_account_parsed_outputs
TODO

## get_account_simple_siglocked
GET outputs locked with simple AddressED25519 lock
`/api/v1/get_account_simple_siglocked?addr=<EasyFL source form of the accountable lock constraint>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_account_simple_siglocked?addr=a(0x24db3c3d477f29d558fbe6f215b0c9d198dcc878866fb60cba023ba3c3d74a03)'
```

```json
{
  "outputs": {
    "0000003d810277c133543f4b79248fb4a0c7b445e44c227d4bddd4d93b5c34a802": "40020b45ad88000088555040760c2345b6a024db3c3d477f29d558fbe6f215b0c9d198dcc878866fb60cba023ba3c3d74a03"
  },
  "lrb_id": "80007f9a0001b96d152c58ba26ede99644735cfa8b8353dff823e925c8f73140"
}
```

## get_outputs_for_amount
TODO

## get_nonchain_balance
TODO


## get_chain_outputs
Get the chain outputs for the provided accountable
`/api/v1/get_chain_outputs?accountable=<EasyFL source form of the accountable lock constraint>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_chain_outputs?accountable=a(0x24db3c3d477f29d558fbe6f215b0c9d198dcc878866fb60cba023ba3c3d74a03)'
```

```json
{
  "outputs": {
    "00007fa40301870f4ed68e631ddef7dfeb2fbc2f1a39241f59303ead0e0afb2200": "40040b45ad880000001748800d4b5955ea810245b6a0aad6a0102e6f51834bf26b6d8367cc424cf78713f59dd3bc6d54eab23ccdee5245b6a024db3c3d477f29d558fbe6f215b0c9d198dcc878866fb60cba023ba3c3d74a03850000003d8188000000174876e8002645c5a311c0a3a0f40215f6bf9d03a45ba7a90fcfb3d44b09582c20aa13fba17cc59a9e0002000d49dd8800000000000019c08102"
  },
  "lrb_id": "80007fa50001a93dbaf389afa9faa00e1236d6d28671cf4a8e3824ae0d891340"
}
```


## get_chain_output
Get the chain output for the provided chain id
`/api/v1/get_chain_output?chainid=<hex-encoded chain ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_chain_output?chainid=6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc'
```

```json
{
  "output_id": "80003cc800019ca4acabc96e3bc390c4278f8ab861c0021aa22106368bfdd4ba00",
  "output_data": "40060b45ab8800038d7f873d1bc52345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c2a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d181028800038d7f873d1bc51d504287626f6f742e62308400000475840000023f8800000000000000006151d7880000000000386580d103b7a35d38fa03bbb8554dcbd8abe2f42903bdc65d4a3d1f13d8bf2d86f2320397dcf8e754a9bf1e1dee3d515b312d25a70f7850d97748f47abc795538eb0a5d6e175dd6741797f18b62ec7d5c23343f59810281ff",
  "lrb_id": "80003cc800019ca4acabc96e3bc390c4278f8ab861c0021aa22106368bfdd4ba"
}
```

## get_output
Get output data for the provided output id
`/api/v1/get_output?id=<hex-encoded output ID>`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_output?id=80003d180001780694657bcff9b6a3ccd0e9146fad6e8692be33e1cf8c1d4c5a00'
```

```json
{
  "output_data": "40060b45ab8800038d7fa6b1266d2345b3a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c2a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d181028800038d7fa6b1266d1d504287626f6f742e62308400000515840000028f8800000000000000006151d7880000000000386580d10219960a241bda3e6475dc3c5ec4902d221f77c62cd8f90f77d212ee7ee7ebe169af56782638a96a8422e5ec87b5cd985e05aed0ef65405ff26fe7bc4a490a96eb767976a3158c29a7c81966de20076c23810281ff",
  "lrb_id": "80003d180001780694657bcff9b6a3ccd0e9146fad6e8692be33e1cf8c1d4c5a"
}
```

## submit_tx
POST transaction bytes
Feedback only on parsing error, otherwise async posting
`/api/v1/submit_tx`

Example:
TODO

## sync_info
GET sync info from the node

`/api/v1/sync_info`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/sync_info'
```

```json
{
  "synced": true,
  "current_slot": 15718,
  "lrb_slot": 15718,
  "ledger_coverage": "2_000_009_657_532_981",
  "per_sequencer": {
    "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc": {
      "synced": true,
      "latest_healthy_slot": 15718,
      "latest_committed_slot": 15718,
      "ledger_coverage": 2000009657532981
    }
  }
}
```


## node_info
GET node info from the node

`/api/v1/node_info`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/node_info'
```

```json
{
  "id": "12D3KooWSEDuiViLCgy6RvzQWeziKk79aAMikGPFnMKjnLzv9TVi",
  "version": "v0.1.3-testnet",
  "num_static_peers": 0,
  "num_dynamic_alive": 0,
  "sequencers": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc"
}
```

## peers_info
GET peers info from the node

`/api/v1/peers_info`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/peers_info'
```

```json
{
  "host_id": "12D3KooWSQWMFg78817tyNFP7GsqUvYp2TQdfXC96bw84nvJrj2Z",
  "peers": [
    {
      "id": "12D3KooWBRmgc5d2kusZ8xtXQ98iK1qtjP6CzWSxSyTeUzMJEZnV",
      "multiAddresses": [
        "/ip4/83.229.84.197/udp/4000/quic-v1"
      ],
      "is_static": false,
      "responds_to_pull": false,
      "is_alive": true,
      "when_added": 1733327100186579692,
      "last_heartbeat_received": 1733329725559708378,
      "clock_differences_quartiles": [
        9680897,
        10180110,
        10511030
      ],
      "hb_differences_quartiles": [
        2000333141,
        2000521070,
        2001048199
      ],
      "num_incoming_hb": 1312,
      "num_incoming_pull": 0,
      "num_incoming_tx": 5070
    }
  ]
}

```

## get_latest_reliable_branch
GET latest reliable branch
`/api/v1/get_latest_reliable_branch`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_latest_reliable_branch'
```

```json
{
  "root_record": {
    "root": "13511d56313d105804a18bb78e7d008bf367fc90c96f20fe84003cad5c41da2b",
    "sequencer_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc",
    "ledger_coverage": 2000010399831606,
    "slot_inflation": 6557224,
    "supply": 1000005215558988
  },
  "branch_id": "80003da0000109a144ad17b0fb53f5378cecad81c93aeb87d1081dd02c6eff18"
}
```

## check_txid_in_lrb
GET latest reliable branch and check if transaction ID is in it
`/api/v1/check_txid_in_lrb?txid=<hex-encoded transaction ID>`

Example:
``` bash
curl -L -X GET 'http://localhost:8000/api/v1/check_txid_in_lrb?txid=8000e1ed00014ff2a17201cd31c0b05e7e63f8ed8a451d6fcaff23d4a0156544'
```

```json
{
  "lrb_id": "8000e1ed00014ff2a17201cd31c0b05e7e63f8ed8a451d6fcaff23d4a0156544",
  "txid": "8000e1ed00014ff2a17201cd31c0b05e7e63f8ed8a451d6fcaff23d4a0156544",
  "included": true
}
```

## last_known_milestones
GET latest known milestone list
`/api/v1/last_known_milestones`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/last_known_milestones'
```

```json
{
  "sequencers": {
    "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc": {
      "latest_milestone_txid": "8000e094190045abb5df0bcb56c40f0aed7186738145c382534b0f6c51220890",
      "last_branch_txid": "8000e094190045abb5df0bcb56c40f0aed7186738145c382534b0f6c51220890",
      "milestone_count": 9,
      "last_activity_unix_nano": 1733757110088545254
    }
  }
}
```

## get_mainchain
GET main chain of branches /get_mainchain?[max=]
`/api/v1/get_mainchain?[max=]`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_mainchain?max=3'
```

```json
{
  "branches": [
    {
      "id": "8000e0ab0001da67ddc750dd991b23ff3ee60311fdbf538eeb1ae6f87d348195",
      "data": {
        "root": {
          "root": "5d1194c327d62b18f3f931dff5f5d70d06455b1ffb1c8cb1a26a105e41e072aa",
          "sequencer_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc",
          "ledger_coverage": 2000012648973651,
          "slot_inflation": 7961922,
          "supply": 1000006349006291
        },
        "stem_output_index": 1,
        "sequencer_output_index": 0,
        "on_chain_amount": 1000006347006791,
        "branch_inflation": 3865922
      }
    },
    {
      "id": "8000e0aa00017f84eb61f9fa57ee53f962cd8fcb960524ebed7feebea5afe2ba",
      "data": {
        "root": {
          "root": "8d332206e59a9fcbab7d67bb8ea44f6011896e771e8fb5623795705e395bb6a8",
          "sequencer_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc",
          "ledger_coverage": 2000012619857565,
          "slot_inflation": 7220170,
          "supply": 1000006341044369
        },
        "stem_output_index": 1,
        "sequencer_output_index": 0,
        "on_chain_amount": 1000006339044869,
        "branch_inflation": 3124170
      }
    },
    {
      "id": "8000e0a90001f30cb3d7cbe27ef332cf3101f35030768d008f49dac3a63693cf",
      "data": {
        "root": {
          "root": "88050479542316255c52603657bc0c4e1414e7d038a4e0f6ccb020c595cd8efb",
          "sequencer_id": "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc",
          "ledger_coverage": 2000012576065733,
          "slot_inflation": 7086286,
          "supply": 1000006333824199
        },
        "stem_output_index": 1,
        "sequencer_output_index": 0,
        "on_chain_amount": 1000006331824699,
        "branch_inflation": 2990286
      }
    }
  ]
}
```

## get_all_chains
GET all chains in the LRB
`/api/v1/get_all_chains`

Example:

``` bash
curl -L -X GET 'http://localhost:8000/api/v1/get_all_chains'
```

```json

{
  "chains": {
    "11c0a3a0f40215f6bf9d03a45ba7a90fcfb3d44b09582c20aa13fba17cc59a9e": {
      "id": "00007fb61c01d1f0a6458fd02e30b43757ca75b67b3feb9af21690c288f4303000",
      "data": "40040b45ad88000000174880dde05955ea810245b6a0aad6a0102e6f51834bf26b6d8367cc424cf78713f59dd3bc6d54eab23ccdee5245b6a024db3c3d477f29d558fbe6f215b0c9d198dcc878866fb60cba023ba3c3d74a03850000003d8188000000174876e8002645c5a311c0a3a0f40215f6bf9d03a45ba7a90fcfb3d44b09582c20aa13fba17cc59a9e0002000d49dd8800000000000019c08102"
    },
    "3862b91b75c881d0f2787d0ad55c1da1ed66cfa932db84e774630ce56d20d7e4": {
      "id": "80007fb71900f36d6cdb90ac153c29ce32c7a06e8d1112395583010dbab9655700",
      "data": "40060b45ad8800007f549671420e2345b6a0aa401c8c6a9deacf479ab2209c07c01a27bd1eeecf0d7eaa4180b8049c6190d02645c5a33862b91b75c881d0f2787d0ad55c1da1ed66cfa932db84e774630ce56d20d7e40002000d49d581028800007f549671420e1d504287736571312e6531840000014084000000388800000000000000000d49dd880000000000466b968102"
    },
    "6393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc": {
      "id": "80007fb719001cba6e861d193b9a048ce49101925c044139dde564b0498e749400",
      "data": "40060b45ad8800016bcaac35db212345b6a0033d48aa6f02b3f37811ae82d9c383855d3d23373cbd28ab94639fdd94a4f02d2645c5a36393b6781206a652070e78d1391bc467e9d9704e9aa59ec7f7131f329d662dcc0002000d49d581028800016bcaac35db211d504287626f6f742e6531840000015a840000003a8800000000000000000d49dd880000000000c9320d8102"
    },
    "795d6449ef9c59a47294d6b339246d092fd98e3ed679ac6755102cf58590a9ea": {
      "id": "80007fb800018c63fb7e200056e0e548fd30e45dd0f3d43481e9215c3b58a17700",
      "data": "40060b45ad8800007f5496c2fc7e2345b6a0aad6a0102e6f51834bf26b6d8367cc424cf78713f59dd3bc6d54eab23ccdee522645c5a3795d6449ef9c59a47294d6b339246d092fd98e3ed679ac6755102cf58590a9ea0002000d49d581028800007f5496c2fc7e1d504287736571342e62308400000161840000003c8800000000000000000d49dd88000000000039d8c38102"
    },
    "d048d81f4330dbebc149b2dafcdbb9ff088c7516ddf41590c1f09a033517dbdc": {
      "id": "80007fb63700de6821f02227cb1701a9597f0ac4c04f8545ea62c7c3dcd5524400",
      "data": "40060b45ad8800007f54955b423c2345b6a062c733803a83a26d4db1ce9f22206281f64af69401da6eb26390d34e6a88c5fa2645c5a3d048d81f4330dbebc149b2dafcdbb9ff088c7516ddf41590c1f09a033517dbdc0002000d49d581028800007f54955b423c1d504287736571322e6532840000013f84000000398800000000000000000d49dd8800000000000000008102"
    }
  },
  "lrb_id": "80007fb800018c63fb7e200056e0e548fd30e45dd0f3d43481e9215c3b58a177"
}

```

## get_delegations_by_sequencer
TODO