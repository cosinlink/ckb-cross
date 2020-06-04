const CKB = require("@nervosnetwork/ckb-sdk-core").default;
const ckb_utils = require("@nervosnetwork/ckb-sdk-utils");

const process = require("process");
const fs = require("fs");
const _ = require("lodash");
const  {blake2b}  = require("./utils")

const nodeUrl = "http://127.0.0.1:8114/";
const ckb = new CKB(nodeUrl);
const simpleUdtBinary = fs.readFileSync("../deps/simple_udt");

const simpleUdtHash = blake2b(simpleUdtBinary);
const privateKey =
    "0xd00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2bc";

const bPrivKey =
    "0xd00c06bfd800d27397002dca6fb0993d5ba6399b4238b2f29ee9deb97593d2b0";
let config = {
    "deployTxHash":""
}

const fee = 100000000n;


async function deploy_sudt() {
    const secp256k1Dep = await ckb.loadSecp256k1Dep();

    // admin
    const publicKey = ckb.utils.privateKeyToPublicKey(privateKey);
    const publicKeyHash = `0x${ckb.utils.blake160(publicKey, "hex")}`;
    const lockScript = {
        hashType: secp256k1Dep.hashType,
        codeHash: secp256k1Dep.codeHash,
        args: publicKeyHash
    };
    const lockHash = ckb.utils.scriptToHash(lockScript);

    // user b
    const bPubKey = ckb.utils.privateKeyToPublicKey(bPrivKey);
    const bPubKeyHash = `0x${ckb.utils.blake160(bPubKey, "hex")}`;
    const bLockScript = {
        hashType: secp256k1Dep.hashType,
        codeHash: secp256k1Dep.codeHash,
        args: bPubKeyHash
    };
    const bLockHash = ckb.utils.scriptToHash(bLockScript);

    const unspentCells = await ckb.loadCells({
        lockHash
    });
    const totalCapacity = unspentCells.reduce(
        (sum, cell) => sum + BigInt(cell.capacity),
        BigInt(0)
    );
    config.udtScript = {
        codeHash: ckb_utils.bytesToHex(simpleUdtHash),
        hashType: "data",
        args: lockHash
    };
    const CellCapacity = 20000000000000n;

    const transaction = {
        version: "0x0",
        cellDeps: [
            {
                outPoint: {
                    txHash: config.deployTxHash,
                    index: "0x0"
                },
                depType: "code"
            },
            {
                outPoint: secp256k1Dep.outPoint,
                depType: "depGroup"
            }
        ],
        headerDeps: [],
        inputs: unspentCells.map(cell => ({
            previousOutput: cell.outPoint,
            since: "0x0"
        })),
        outputs: [
            {
                lock: lockScript,
                type: null,
                capacity: "0x" + (totalCapacity - fee - CellCapacity).toString(16)
            },
            {
                lock: bLockScript,
                type: config.udtScript,
                capacity: "0x" + CellCapacity.toString(16)
            }
        ],
        witnesses: [
            {
                lock: "",
                inputType: "",
                outputType: ""
            }
        ],
        outputsData: [
            "0x",
            ckb_utils.toHexInLittleEndian("0x" + Number(100000000).toString(16), 16)
        ]
    };
    const signedTransaction = ckb.signTransaction(privateKey)(transaction);
    //   console.log(JSON.stringify(signedTransaction, null, 2))

    const txHash = await ckb.rpc.sendTransaction(
        signedTransaction,
        "passthrough"
    );
    config.issueTxHash = txHash;
    console.log(`issue sudt hash: ${txHash}`);
}


async function main() {
    await deploy_sudt()
    await waitForTx(config.deployTxHash);
}

main()

async function waitForTx(txHash) {
    while (true) {
        const tx = await ckb.rpc.getTransaction(txHash);
        try {
            console.log(`tx ${txHash} status: ${tx.txStatus.status}`);
            if (tx.txStatus.status === "committed") {
                return;
            }
        } catch (e) {
            console.log({ e, tx, txHash });
        }
        await delay(1000);
    }
}
