axis:
	go run examples/main.go -password $(PASSWORD_AXIS)  ../statement_parser/examples/axis.pdf

uco:
	go run examples/main.go -password $(PASSWORD_UCO) ../statement_parser/examples/uco.pdf

hdfc:
	go run examples/main.go -password $(PASSWORD_HDFC) ../statement_parser/examples/hdfc.pdf

hdfc-com:
	go run examples/main.go -password $(PASSWORD_HDFC) ../statement_parser/examples/hdfc-com.pdf

sbi:
	go run examples/main.go -password $(PASSWORD_SBI) ../statement_parser/examples/sbi2.pdf

clean:
	rm -rf output

.PHONY: axis clean
