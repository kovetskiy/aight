package main

//type PatchFileArguments struct {
//    Path         string `json:"path"`
//    Index        int    `json:"index"`
//    Size         int    `json:"size"`
//    Substitution string `json:"substitution"`
//}

//func (dispatcher *Dispatcher) patchFile(args PatchFileArguments) (any, error) {
//    path, err := dispatcher.sandbox(args.Path)
//    if err != nil {
//        return err, nil
//    }

//    contents, err := os.ReadFile(path)
//    if err != nil {
//        return karma.Format(err, "read file"), nil
//    }

//    if args.Index > len(contents) {
//        return fmt.Errorf(
//            "position %d is out of range (length: %d)",
//            args.Index,
//            len(contents),
//        ), nil
//    }

//    if args.Index+args.Size > len(contents) {
//        return fmt.Errorf(
//            "length %d is out of range (length: %d)",
//            args.Size,
//            len(contents),
//        ), nil
//    }

//    buffer := bytes.NewBuffer(contents[:args.Index])
//    buffer.WriteString(args.Substitution)
//    buffer.Write(contents[args.Index+args.Size:])

//    fd, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
//    if err != nil {
//        return karma.Format(err, "open file"), nil
//    }

//    defer fd.Close()

//    _, err = fd.Write(buffer.Bytes())
//    if err != nil {
//        return karma.Format(err, "write file"), nil
//    }

//    return true, nil
//}
