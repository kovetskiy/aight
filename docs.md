Transactions
To fetch all someone transaction history 'etherscan' offer just a one way: https://etherscan.io/address/0xAAAsomeADDR00000000000 and here you'll see web interface to comfortable read transaction list.. but if you looking for approach to fetch all them as json with your application there is one API:

GET: http://api.etherscan.io/api?module=account&action=txlist&address=0xAAAsomeADDR00000000000&sort=asc

Example of a response:

{
  "status": "1",
  "message": "OK",
  "result": [
    {
      "blockNumber": "------",
      "timeStamp": "-------",
      "hash": "0x-----",
      "nonce": "1",
      "blockHash": "0x--------",
      "transactionIndex": "70",
      "from": "0x----------",
      "to": "0x---------",
      "value": "87000",
      "gas": "636879",
      "gasPrice": "10000000000",
      "isError": "0",
      "txreceipt_status": "1",
      "input": ".. loooong value ...",
      "contractAddress": "0x--------",
      "cumulativeGasUsed": "3190544",
      "gasUsed": "636879",
      "confirmations": "-----"
    },
    ...
  ]
}
Pay attention a value gives in wei and you might convert it to eth before use it in your processes, you albe to do it by formula eth = wei / (10 * 10^17) and validate your result here for example

ฅ(≚ᄌ≚) Have a nice day!

