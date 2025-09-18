# Building Go HTMX app

curl -X POST "http://localhost:5000/subjects/ai"   -H "Content-Type: text/plain"   --data-binary "hello world"
curl -X POST "http://localhost:7777/users/1/subjects/ai" \
  -H "Content-Type: text/plain" \
  --data-binary 'neural networks are everywhere!


[{"UserID":"1","Subjects":[{"Name":"ai","Thoughts":["neural networks are everywhere!"]}]}]
